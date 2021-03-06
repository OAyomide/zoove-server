package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"zoove/controllers"
	"zoove/db"
	"zoove/middleware"
	"zoove/platforms"
	"zoove/types"
	"zoove/util"

	"github.com/gofiber/websocket/v2"
	"github.com/soveran/redisurl"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	jwtware "github.com/gofiber/jwt/v2"
	"github.com/gomodule/redigo/redis"
	"github.com/joho/godotenv"
)

var pool *redis.Pool
var register = make(chan *websocket.Conn)
var jaegerChan = make(chan *SocketMessage)
var spotifyChan = make(chan *types.SingleTrack)
var deezerChan = make(chan *types.SingleTrack)
var createPlaylistChan = make(chan bool)

func loadEnv() {
	envr := os.Getenv("ENV")
	err := godotenv.Load(".env." + envr)
	if err != nil {
		log.Println("Error reading the env file")
		log.Println(err)
		// panic(err)
	}
}

func init() {
	loadEnv()
}

// SocketMessage represents an incoming socket message
type SocketMessage struct {
	Type    string `json:"action_type"`
	URL     string `json:"url"`
	Payload struct {
		Title    string   `json:"title"`
		Tracks   []string `json:"tracks"`
		Platform string   `json:"platform"`
	} `json:"payload,omitempty"`
	UserID string `json:"userid,omitempty"`
}

func loadListeners() {
	for {
		select {
		case <-register:
		}
	}
}

// SocketListener represents a "blueprint" for a typical listener
type SocketListener struct {
	deserialize   SocketMessage
	c             *websocket.Conn
	trackMeta     *types.SingleTrack
	deezerTracks  []types.SingleTrack
	spotifyTracks []types.SingleTrack
	tracks        [][]types.SingleTrack
	client        *db.PrismaClient
	playlistMeta  *types.Playlist
}

// GetTrackListener listens for tracks action
func (listener *SocketListener) GetTrackListener() {
	// log.Println("Deserialized extracted URL (TRACK) is: ", listener.deserialize.URL)
	extracted, err := util.ExtractInfoMetadata(listener.deserialize.URL)
	if err != nil {
		log.Println("Error extracting")
		log.Println(err)
		listener.c.WriteMessage(websocket.TextMessage, []byte(`{"desc":"error", "message":"Its me not you...."`))
		listener.c.Close()
	}
	if extracted.Host == util.HostDeezer {
		// log.Println("Wants to search deezer")
		listener.trackMeta, err = platforms.HostDeezerGetSingleTrack(extracted.ID, pool)
		if err != nil {
			listener.c.WriteMessage(websocket.TextMessage, []byte(`{"desc":"Error getting deezer single track"}`))
			listener.c.Close()
		}

	} else if extracted.Host == util.HostSpotify {
		// log.Println("Wants to search spotify")
		listener.trackMeta, err = platforms.HostSpotifyGetSingleTrack(extracted.ID, pool)
		if err != nil {
			listener.c.WriteMessage(websocket.TextMessage, []byte(`{"desc":"Error getting spotify single track"}`))
			listener.c.Close()
		}
	} else {
		log.Println("Oops! Not a valid host")
		listener.c.WriteMessage(websocket.TextMessage, []byte(`{"desc":"Invalid host"}`))
		listener.c.Close()
		return
	}
	artiste := ""
	if len(listener.trackMeta.Artistes) > 0 {
		artiste = listener.trackMeta.Artistes[0]
	}
	search := platforms.NewTrackToSearch(listener.trackMeta.Title, artiste, pool)
	deezr, err := search.HostDeezerSearchTrack()
	if err != nil {
		log.Println("Error searching deezer")
		// log.Println("Error is: ", err)
		// TODO: try to handle whatever happens here
		deezr = &types.SingleTrack{}
	}

	spot, err := search.HostSpotifySearchTrack()
	if err != nil {
		// log.Println("Errpr searching spotify")
		// TODO: try to handle whatever happens here
		spot = &types.SingleTrack{}
	}
	conn := pool.Get()
	defer conn.Close()

	_, err = redis.String(conn.Do("GET", util.RedisSearchesKey))
	if err != nil {
		if err == redis.ErrNil {
			_, err := redis.String(conn.Do("SET", util.RedisSearchesKey, "1"))
			if err != nil {
				log.Println("Error saving searches key into redis")
			}
		}
	}

	searchesCount, err := redis.Int(conn.Do("INCR", util.RedisSearchesKey))
	if err != nil {
		log.Println("Error incrementing redis key")
	}
	log.Printf("Number of search so far: %d\n", searchesCount)
	deezr.ReleaseDate = spot.ReleaseDate
	listener.deezerTracks = append(listener.deezerTracks, *deezr)
	listener.spotifyTracks = append(listener.spotifyTracks, *spot)
	listener.tracks = append(listener.tracks, listener.spotifyTracks, listener.deezerTracks)
	listener.c.WriteJSON(listener.tracks)

	// we gotta reset those values, else, it'd just keep pushing to the arrays and returning increasing values as the user makes more requests
	// perhaps have @Davidemi to review this for me.
	listener.tracks = nil
	listener.deezerTracks = nil
	listener.spotifyTracks = nil
	listener.c.Close()
}

// GetPlaylistListener returns the playlist listener
func (listener *SocketListener) GetPlaylistListener() {
	// log.Println("Deserialized extracted URL (playlist) is: ", listener.deserialize.URL)
	extracted, err := util.ExtractInfoMetadata(listener.deserialize.URL)
	if err != nil {
		log.Println("Error extracting")
		log.Println(err)
		listener.c.WriteMessage(websocket.TextMessage, []byte(`{"desc":"error", "message":"Its me not you...."`))
		listener.c.Close()
	}

	if extracted.Host == util.HostDeezer {
		deezerPl, err := platforms.HostDeezerFetchPlaylistTracks(extracted.ID, pool)
		if err != nil {
			log.Println("Error fetching playlist tracks.")
			log.Println(err)
			if err.Error() == "Not Found" {
				listener.playlistMeta = &types.Playlist{}
			}
		}

		listener.playlistMeta = &deezerPl

		for _, singleTrack := range listener.playlistMeta.Tracks {
			search := platforms.NewTrackToSearch(singleTrack.Title, singleTrack.Artistes[0], pool)
			go search.HostSpotifySearchTrackChan(spotifyChan)
			spotifyTrack := <-spotifyChan

			if spotifyTrack == nil {
				continue
			}

			listener.spotifyTracks = append(listener.spotifyTracks, *spotifyTrack)
		}

		listener.deezerTracks = append(listener.deezerTracks, listener.playlistMeta.Tracks...)

	} else if extracted.Host == util.HostSpotify {
		spotifyPl, err := platforms.HostSpotifyFetchPlaylistTracks(extracted.ID, pool)
		if err != nil {
			log.Println("Error fetching spotify playlist tracks.")
		}
		listener.playlistMeta = &spotifyPl

		for _, singleTrack := range listener.playlistMeta.Tracks {
			artiste := ""
			if len(singleTrack.Artistes) > 0 {
				artiste = singleTrack.Artistes[0]
			}

			search := platforms.NewTrackToSearch(singleTrack.Title, artiste, pool)
			go search.HostDeezerSearchTrackChan(deezerChan)
			deezerTrack := <-deezerChan
			if deezerTrack == nil {
				continue
			}
			listener.deezerTracks = append(listener.deezerTracks, *deezerTrack)
		}
		listener.spotifyTracks = append(listener.spotifyTracks, listener.playlistMeta.Tracks...)
	}

	conn := pool.Get()
	defer conn.Close()

	_, err = redis.String(conn.Do("GET", util.RedisSearchesKey))
	if err != nil {
		if err == redis.ErrNil {
			_, err := redis.String(conn.Do("SET", util.RedisSearchesKey, "1"))
			if err != nil {
				log.Println("Error saving searches key into redis")
			}
		}
	}

	searchesCount, err := redis.Int(conn.Do("INCR", util.RedisSearchesKey))
	if err != nil {
		log.Println("Error incrementing redis key")
	}
	log.Printf("Number of search so far: %d\n", searchesCount)

	diff := 0
	if len(listener.deezerTracks) > len(listener.spotifyTracks) {
		diff = len(listener.deezerTracks) - len(listener.spotifyTracks)
		listener.deezerTracks = listener.deezerTracks[:len(listener.deezerTracks)-diff]
	} else if len(listener.spotifyTracks) > len(listener.deezerTracks) {
		diff = len(listener.spotifyTracks) - len(listener.spotifyTracks)
		listener.spotifyTracks = listener.spotifyTracks[:len(listener.spotifyTracks)-diff]
	}

	for index, single := range listener.deezerTracks {
		single.ReleaseDate = listener.spotifyTracks[index].ReleaseDate
	}

	log.Println("Final deezer tracks are: ", listener.deezerTracks)
	listener.tracks = append(listener.tracks, listener.deezerTracks, listener.spotifyTracks)
	// log.Println("All tracks now are: ", listener.tracks)
	// log.Println("Plalyist meta is: ", listener.playlistMeta)
	res := map[string]interface{}{
		"playlist_title": listener.playlistMeta.Title,
		"payload":        listener.tracks,
		"owner":          listener.playlistMeta.Owner,
		"playlist_meta":  listener.playlistMeta,
		"platforms": map[string]interface{}{
			"spotify": listener.spotifyTracks,
			"deezer":  listener.deezerTracks,
		},
	}

	listener.c.WriteJSON(res)
	listener.deezerTracks = nil
	listener.spotifyTracks = nil
	listener.tracks = nil
	listener.c.Close()
}

// CreatePlaylistListener creates a playlist for a user.
func (listener *SocketListener) CreatePlaylistListener() {
	existing, _ := listener.client.User.FindOne(db.User.PlatformID.Equals(listener.deserialize.UserID)).Exec(context.Background())
	go platforms.CreatePlaylistChan(existing.PlatformID, listener.deserialize.Payload.Title, existing.Token, listener.deserialize.Payload.Platform, listener.deserialize.Payload.Tracks, createPlaylistChan)
	_ = <-createPlaylistChan
	res := map[string]interface{}{
		"action":  "create",
		"payload": true,
	}

	listener.c.WriteJSON(res)
	listener.c.Close()
}

func main() {
	app := fiber.New()

	client := db.NewClient()
	err := client.Connect()

	if err != nil {
		log.Println("Error creating new DB connection")
		log.Fatalln(err)
	}

	defer func() {
		err := client.Disconnect()
		if err != nil {
			log.Fatalln(err)
		}
	}()

	userHandler := controllers.NewUserHandler(client, pool)
	jaeger := controllers.NewJaeger(pool)
	authentication := middleware.NewAuthUserMiddleware(client)

	go loadListeners()

	app.Use(cors.New(cors.Config{
		AllowMethods: fmt.Sprintf("%s,%s,%s,%s,%s", http.MethodGet, http.MethodPatch, http.MethodPost, http.MethodOptions, http.MethodDelete),
		AllowOrigins: "*",
	}))

	type Sample struct {
		AccessToken string `query:"access_token"`
	}

	app.Get("/deezer/channel.html", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).SendFile("./channel.html")
	})

	app.Get("/:platform/join", func(c *fiber.Ctx) error {
		platform := c.Params("platform")
		log.Print("User is trying to join or login")
		log.Println(platform)
		if platform == util.HostDeezer {

			DeezerAuthBase := os.Getenv("DEEZER_AUTH_BASE")
			DeezerAppID := os.Getenv("DEEZER_APP_ID")
			DeezerRedirectURI := os.Getenv("DEEZER_REDIRECT_URI")
			scopes := "basic_access,email,offline_access,listening_history,manage_library"
			url := fmt.Sprintf("%s/auth.php?app_id=%s&redirect_uri=%s&perms=%s", DeezerAuthBase, DeezerAppID, DeezerRedirectURI, scopes)
			return c.Redirect(url)
		} else if platform == util.HostSpotify {
			spotifyAuthBase := os.Getenv("SPOTIFY_AUTH_BASE")
			spotifyAppID := os.Getenv("SPOTIFY_APP_ID")
			spotifyRedirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")
			scopes := url.QueryEscape("user-read-private user-read-email playlist-modify-public playlist-modify-private user-library-modify user-top-read user-read-recently-played user-read-currently-playing")
			url := fmt.Sprintf("%s/authorize?response_type=code&client_id=%s&scope=%s&redirect_uri=%s", spotifyAuthBase, spotifyAppID, scopes, spotifyRedirectURI)
			return c.Redirect(url)
		}
		return util.NotImplementedError(c, nil)
	})

	app.Use("/api/v1.1/ws", func(ctx *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			ctx.Locals("allowed", true)
			return ctx.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/api/v1.1/ws/connect", websocket.New(func(c *websocket.Conn) {
		var tracks = [][]types.SingleTrack{}
		var deezerTracks = []types.SingleTrack{}
		var spotifyTracks = []types.SingleTrack{}
		pool = &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redisurl.Connect()
			},
		}

		register <- c
		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Println("Read Error:", err)
				}
				return
			}

			deserialize := &SocketMessage{}
			err = json.Unmarshal(msg, deserialize)
			if err != nil {
				log.Println("Error parsing. Seems client is sending non-json data")
				c.WriteMessage(websocket.TextMessage, []byte(`{"desc":"send JSON unmarshalling errors here"}`))
				c.Close()
			}

			var trackMeta = &types.SingleTrack{}
			var playlistMeta = &types.Playlist{}
			listener := &SocketListener{deserialize: *deserialize,
				c: c, client: client, deezerTracks: deezerTracks,
				playlistMeta:  playlistMeta,
				spotifyTracks: spotifyTracks,
				trackMeta:     trackMeta,
				tracks:        tracks,
			}
			if deserialize.Type == "track" {
				listener.GetTrackListener()
			} else if deserialize.Type == "playlist" {
				listener.GetPlaylistListener()
			} else if deserialize.Type == "create_playlist" {
				listener.CreatePlaylistListener()
			} else {
				c.Close()
			}
		}
	}))
	app.Get("/:platform/signup", userHandler.SignupRedirect)
	app.Get("/deezer/verify", userHandler.VerifyDeezerSignup)
	app.Get("/kanye/:platform/oauth", userHandler.AuthorizeUser)
	app.Post("/api/v1.1/user/join", userHandler.AddNewUser)
	app.Use(middleware.ExtractedInfoMiddleware)
	app.Get("/api/v1.1/search", jaeger.JaegerHandler)
	app.Get("/api/v1.1/zoovify/playlist", jaeger.ConvertPlaylist)

	app.Use(jwtware.New(
		jwtware.Config{SigningKey: []byte(os.Getenv("JWT_SECRET")),
			Claims:     &types.Token{},
			ContextKey: "user",
		}))
	app.Use(authentication.AuthenticateUser)
	app.Get("/api/v1.1/me", userHandler.GetUserProfile)
	app.Get("/api/v1.1/me/update", userHandler.UpdateUserProfile)
	app.Get("/api/v1.1/me/history", userHandler.GetListeningHistory)
	app.Get("/api/v1.1/me/history/artistes", userHandler.GetArtistePlayHistory)

	// app.Get("/api/v1.1/me/history")

	port := os.Getenv("PORT")
	if port == "" {
		port = "13200"
	}

	port = fmt.Sprintf(":%s", port)
	app.Listen(port)
}
