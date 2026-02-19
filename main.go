package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

type Album struct {
	ID            int    `json:"ID"`
	Rank          int    `json:"Rank"`
	Title         string `json:"Title"`
	Band          string `json:"Band"`
	Country       string `json:"Country"`
	Year          int    `json:"Year"`
	TimesListened int    `json:"TimesListened"`
}

var db *sql.DB

func initDB() {
	// Crear carpeta data si no existe
	if _, err := os.Stat("data"); os.IsNotExist(err) {
		os.Mkdir("data", 0755)
	}

	var err error
	db, err = sql.Open("sqlite", "./data/albums.db")
	if err != nil {
		log.Fatal(err)
	}

	// Crear tabla si no existe
	createTable := `
	CREATE TABLE IF NOT EXISTS albums (
		id INTEGER PRIMARY KEY,
		rank INTEGER,
		title TEXT,
		band TEXT,
		country TEXT,
		year INTEGER,
		times_listened INTEGER DEFAULT 0
	);
	`
	if _, err := db.Exec(createTable); err != nil {
		log.Fatal(err)
	}
}

func loadCSVIntoDB(path string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	tx, _ := db.Begin()
	stmt, _ := tx.Prepare(`
	INSERT OR IGNORE INTO albums (id, rank, title, band, country, year, times_listened)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	defer stmt.Close()

	for i, row := range rows {
		if i == 0 {
			continue // saltar cabecera
		}
		rank, _ := strconv.Atoi(row[0])
		year, _ := strconv.Atoi(row[4])
		stmt.Exec(i, rank, row[1], row[2], row[3], year, 0)
	}

	tx.Commit()
}

func getAllAlbums() ([]Album, error) {
	rows, err := db.Query("SELECT id, rank, title, band, country, year, times_listened FROM albums ORDER BY rank ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []Album
	for rows.Next() {
		var a Album
		if err := rows.Scan(&a.ID, &a.Rank, &a.Title, &a.Band, &a.Country, &a.Year, &a.TimesListened); err != nil {
			return nil, err
		}
		albums = append(albums, a)
	}
	return albums, nil
}

func getRandomUnlistenedAlbum() (*Album, error) {
	row := db.QueryRow("SELECT id, rank, title, band, country, year, times_listened FROM albums WHERE times_listened = 0 ORDER BY RANDOM() LIMIT 1")
	var a Album
	err := row.Scan(&a.ID, &a.Rank, &a.Title, &a.Band, &a.Country, &a.Year, &a.TimesListened)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func markAlbumListened(id int) (*Album, error) {
	_, err := db.Exec("UPDATE albums SET times_listened = times_listened + 1 WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	row := db.QueryRow("SELECT id, rank, title, band, country, year, times_listened FROM albums WHERE id = ?", id)
	var a Album
	if err := row.Scan(&a.ID, &a.Rank, &a.Title, &a.Band, &a.Country, &a.Year, &a.TimesListened); err != nil {
		return nil, err
	}
	return &a, nil
}

func resetAllAlbums() error {
	_, err := db.Exec("UPDATE albums SET times_listened = 0")
	return err
}

func main() {
	rand.Seed(int64(os.Getpid()))

	initDB()
	loadCSVIntoDB("data/albums.csv") // Carga inicial, si no existen registros

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	r.GET("/", func(c *gin.Context) {
		albums, _ := getAllAlbums()
		albumsJSON, _ := json.Marshal(albums)
		c.HTML(http.StatusOK, "index.html", gin.H{
			"albumsJS": template.JS(albumsJSON),
		})
	})

	r.POST("/random", func(c *gin.Context) {
		album, _ := getRandomUnlistenedAlbum()
		if album == nil {
			c.JSON(200, gin.H{"message": "Todos los álbumes ya se escucharon"})
			return
		}
		c.JSON(200, album)
	})

	r.POST("/mark-listened/:id", func(c *gin.Context) {
		idParam := c.Param("id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			c.JSON(400, gin.H{"error": "ID inválido"})
			return
		}
		album, err := markAlbumListened(id)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, album)
	})

	r.POST("/reset", func(c *gin.Context) {
		if err := resetAllAlbums(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "Lista reiniciada"})
	})

	r.Run(":8080")
}
