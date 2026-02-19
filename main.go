package main

import (
	"encoding/csv"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

type Album struct {
	ID            int
	Name          string
	Artist        string
	TimesListened int
	Rating        int
	Comment       string
}

var albums []Album

func loadAlbumsFromCSV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i, row := range rows {
		if i == 0 {
			continue
		} // header
		albums = append(albums, Album{
			ID:     i,
			Name:   row[0],
			Artist: row[1],
		})
	}
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
	rand.Seed(time.Now().UnixNano())
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	if err := loadAlbumsFromCSV("data/albums.csv"); err != nil {
		panic(err)
	}

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{"albums": albums})
	})

	r.POST("/random", func(c *gin.Context) {
		album := getRandomUnlistenedAlbum()
		if album == nil {
			c.JSON(200, gin.H{"message": "Todos los álbumes ya se escucharon"})
			return
		}
		album.TimesListened++
		c.JSON(200, album)
	})

	r.POST("/reset", func(c *gin.Context) {
		for i := range albums {
			albums[i].TimesListened = 0
		}
		c.JSON(200, gin.H{"message": "Lista reiniciada"})
	})

	r.Run(":8080")
}
