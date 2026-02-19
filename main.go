package main

import (
	"encoding/csv"
	"encoding/json"
	"html/template"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Album struct {
	ID            int
	Number        int
	Year          int
	Name          string
	Artist        string
	Genre         string
	Subgenre      string
	TimesListened int
}

var albums []Album

// Carga los álbumes desde el CSV
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
		number, _ := strconv.Atoi(row[0])
		year, _ := strconv.Atoi(row[1])
		albums = append(albums, Album{
			ID:       i,
			Number:   number,
			Year:     year,
			Name:     row[2],
			Artist:   row[3],
			Genre:    row[4],
			Subgenre: row[5],
		})
	}
	return nil
}

// Devuelve un álbum aleatorio que no haya sido escuchado
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
	rand.Seed(time.Now().UnixNano())

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	if err := loadAlbumsFromCSV("data/albums.csv"); err != nil {
		panic(err)
	}

	// Página principal
	r.GET("/", func(c *gin.Context) {
		albumsJSON, _ := json.Marshal(albums)
		c.HTML(http.StatusOK, "index.html", gin.H{
			"albumsJS": template.JS(albumsJSON), // pasa JSON seguro a JS
		})
	})

	// Random album
	r.POST("/random", func(c *gin.Context) {
		album := getRandomUnlistenedAlbum()
		if album == nil {
			c.JSON(200, gin.H{"message": "Todos los álbumes ya se escucharon"})
			return
		}
		c.JSON(200, album)
	})

	// Marcar álbum como escuchado
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

	// Reset de escuchados
	r.POST("/reset", func(c *gin.Context) {
		for i := range albums {
			albums[i].TimesListened = 0
		}
		c.JSON(200, gin.H{"message": "Lista reiniciada"})
	})

	r.Run(":8080") // ejecuta en localhost:8080
}
