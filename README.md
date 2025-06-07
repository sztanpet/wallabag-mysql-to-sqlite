# **Wallabag MariaDB to SQLite Migration**

This Go program migrates your Wallabag data from MariaDB to SQLite. It dynamically handles schema, converts types, and cleans string fields, preserving newlines.

## **Prerequisites**

* **Go (Golang) 1.16+**: [Install Go](https://golang.org/doc/install)  
* **MariaDB/MySQL**: Your source Wallabag database.  

## **Generate up-to-date wallabag sqlite schema **

1. Configure Wallabag for SQLite:  
   Edit docker-compose.yml:  
   * Set SYMFONY\_\_ENV\_\_DATABASE\_URL to sqlite:///%kernel.project\_dir%/data/db/wallabag.sqlite.  
   * Ensure volumes includes ./wallabag\_data:/var/www/wallabag/data.  
   * Remove or comment out the MariaDB (db) service.  
2. **Launch Wallabag (Creates SQLite File):**  
   docker-compose up \-d wallabag

   This generates an empty wallabag.sqlite in wallabag\_data/db/. Stop Wallabag after creation: docker-compose stop wallabag.

## **Migration Tool Setup & Run**

1. Clone Migration Repository:  
   Assume this Go code is in a separate GitHub repository. Clone it:  
   git clone this repository  

2. Copy Generated SQLite File:  
   Copy the wallabag.sqlite file (from Wallabag's wallabag\_data/db/) into this migration project directory:  
   cp /path/to/your\_wallabag\_repo/wallabag\_data/db/wallabag.sqlite ./wallabag.sqlite

3. Configure Connection Details:  
   Open migrate.go and update mariaDBConnStr, sqliteDBPath (to ./wallabag.sqlite), and mariaDBDatabaseName with your MariaDB credentials and database name.  
4. Stop All Database Services:  
   Crucially, stop both MariaDB and Wallabag containers before running the migration:  
   docker-compose down \# from your wallabag repo directory

5. Build and Run Migration:  
   From this migration project directory:  
   go build
   ./migrate-from-mariadb-to-sqlite
   Monitor the logs for completion.

## **Verification**

1. Copy Migrated SQLite File Back:  
   Copy the populated wallabag.sqlite from this migration project back to Wallabag's wallabag\_data/db/:  
   cp ./wallabag.sqlite /path/to/your\_wallabag\_repo/wallabag\_data/db/wallabag.sqlite
   (don't forget to set the owner/group to the original one)

2. Start Wallabag:  
   From your Wallabag repository directory:  
   docker-compose up \-d wallabag

3. Access Wallabag:  
   Open Wallabag in your browser, log in, and verify your data.
