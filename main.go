package main

import (
	"encoding/csv"
	"encoding/json"
	"html/template"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Album struct {
	ID            int
	Rank          int
	Title         string
	Band          string
	Country       string
	Year          int
	TimesListened int
}

var albums []Album

func loadAlbumsFromCSV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i, row := range rows {
		if i == 0 { // saltar cabecera
			continue
		}

		rank, _ := strconv.Atoi(row[0])
		year, _ := strconv.Atoi(row[4])

		// Ignorar filas vacías
		if row[1] == "" || row[2] == "" {
			continue
		}

		albums = append(albums, Album{
			ID:      i,
			Rank:    rank,
			Title:   row[1],
			Band:    row[2],
			Country: row[3],
			Year:    year,
		})
	}

	// Ordenar por Rank
	sort.Slice(albums, func(i, j int) bool {
		return albums[i].Rank < albums[j].Rank
	})

	return nil
}

func getRandomUnlistenedAlbum() *Album {
	var unlistened []*Album
	for i := range albums {
		if albums[i].TimesListened == 0 {
			unlistened = append(unlistened, &albums[i])
		}
	}
	if len(unlistened) == 0 {
		return nil
	}
	idx := rand.Intn(len(unlistened))
	return unlistened[idx]
}

func main() {
	rand.Seed(int64(os.Getpid()))

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	if err := loadAlbumsFromCSV("data/albums.csv"); err != nil {
		panic(err)
	}

	r.GET("/", func(c *gin.Context) {
		albumsJSON, _ := json.Marshal(albums)
		c.HTML(http.StatusOK, "index.html", gin.H{
			"albumsJS": template.JS(albumsJSON),
		})
	})

	r.POST("/random", func(c *gin.Context) {
		album := getRandomUnlistenedAlbum()
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

		for i := range albums {
			if albums[i].ID == id {
				albums[i].TimesListened++
				c.JSON(200, albums[i])
				return
			}
		}
		c.JSON(404, gin.H{"error": "Álbum no encontrado"})
	})

	r.POST("/reset", func(c *gin.Context) {
		for i := range albums {
			albums[i].TimesListened = 0
		}
		c.JSON(200, gin.H{"message": "Lista reiniciada"})
	})

	r.Run(":8080")
}
