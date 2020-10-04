package util

import (
	"errors"
	"log"
	"net/http"
	"time"
	"zoove/types"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
)

const (
	// HostDeezer simply means deezer
	HostDeezer = "deezer"
	// HostSpotify means spotify
	HostSpotify                             = "spotify"
	RedisSearchesKey                        = "searches"
	HostDeezerBasicAccessPermission         = "basic_access"
	HostDeezerEmailPermission               = "email"
	HostDeezerOfflineAccessPermission       = "offline_access"
	HostDeezerManageLibraryAccessPermission = "manage_library"
	HostDeezerManageCommunityPermission     = "manage_community"
	HostDeezerDeleteLibraryPermission       = "delete_library"
	HostDeezerListeningHistoryPermission    = "listening_history"
)

// RequestOk sends back a statusOk response to the client.
func RequestOk(ctx *fiber.Ctx, data interface{}) error {
	return ctx.Status(http.StatusOK).JSON(fiber.Map{"data": data, "message": "Resource found", "error": nil, "status": http.StatusOK})
}

// BadRequest sends back a statusReqBad response to the client
func BadRequest(ctx *fiber.Ctx, err error) error {
	return ctx.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "The request you send is bad", "error": err.Error(), "status": http.StatusBadRequest, "data": nil})
}

// RequestUnAuthorized sends back a statusUnAuthorized to the client
func RequestUnAuthorized(ctx *fiber.Ctx, err error) error {
	return ctx.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "The request you made is unauthorized", "error": err.Error(), "status": http.StatusUnauthorized, "data": nil})
}

// RequestCreated sends back a statusCreated to the client
func RequestCreated(ctx *fiber.Ctx, data interface{}) error {
	return ctx.Status(http.StatusCreated).JSON(fiber.Map{"message": "The resource has been created", "error": nil, "status": http.StatusCreated, "data": data})
}

// NotFound sends back a statusNotFound response to the client
func NotFound(ctx *fiber.Ctx) error {
	return ctx.Status(http.StatusNotFound).JSON(fiber.Map{"message": "The resource does not exist", "error": nil, "status": http.StatusNotFound, "data": nil})
}

// InternalServerError returns an error 500
func InternalServerError(ctx *fiber.Ctx, err error) error {
	return ctx.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "Internal Server Error", "error": err, "status": http.StatusInternalServerError, "data": nil})
}

// NotImplementedError returns a not implemented error
func NotImplementedError(ctx *fiber.Ctx, err error) error {
	return ctx.Status(http.StatusNotImplemented).JSON(fiber.Map{"message": "Not yet implemented", "error": err, "status": http.StatusNotImplemented, "data": nil})
}

// SignJwtToken signs the token that is returned for a user
func SignJwtToken(claims *types.Token, secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &types.Token{
		PlatformToken: claims.PlatformToken,
		Platform:      claims.Platform,
		UUID:          claims.UUID,
		PlatformID:    claims.PlatformID,
		// StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Minute * 3).Unix()},
	})

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// SignJwtTokenExp signs the token that is returned for a user but sets the expiration to 5 mins
func SignJwtTokenExp(claims *types.Token, secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &types.Token{
		PlatformToken:  claims.PlatformToken,
		Platform:       claims.Platform,
		UUID:           claims.UUID,
		PlatformID:     claims.PlatformID,
		StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Minute * 5).Unix()},
	})

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// ParseJwtToken parses a jwt and returns the claims
func ParseJwtToken(value, secret string) (*types.Token, error) {
	tk := &types.Token{}
	tok, err := jwt.ParseWithClaims(value, tk, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("invalid signing method")
		}
		return []byte(secret), nil
	})

	// log.Printf("User token is valid: %v\n", token.Valid)
	// tp, _ := err.(*jwt.ValidationError)
	// log.Printf("%#v\n", tp.Error())
	if err != nil {
		log.Printf("err: %#v\n", err.Error())
		return nil, errors.New("malformed authorization token")
	}
	if !tok.Valid {
		return nil, errors.New("malformed or invalid authorization token")
	}
	return tk, nil
}

// EncryptRefreshToken encrypts a refreshToken for a user
func EncryptRefreshToken(refreshToken string) {}

// TODO: implement refresh token encryption
