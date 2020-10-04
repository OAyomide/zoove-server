# Migration `20200928202831-another-banger`

This migration has been generated by Ayomide Onigbinde <Onigbindeayomide@gmail.com> at 9/28/2020, 9:28:31 PM.
You can check out the [state of the schema](./schema.prisma) after the migration.

## Database Steps

```sql
ALTER TABLE "public"."User" ADD COLUMN "platform" text   NOT NULL ,
ADD COLUMN "avatar" text   NOT NULL 
```

## Changes

```diff
diff --git schema.prisma schema.prisma
migration 20200928200500-another-banger..20200928202831-another-banger
--- datamodel.dml
+++ datamodel.dml
@@ -1,6 +1,6 @@
 datasource postgresql {
-  url = "***"
+  url = "***"
   provider = "postgresql"
 }
 generator db {
@@ -19,5 +19,7 @@
   lang      String
   uuid      String   @unique
   email     String   @unique
   username  String   @unique
+  platform  String
+  avatar    String
 }
```

