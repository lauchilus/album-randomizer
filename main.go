package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite" // driver sqlite puro Go
)

type Album struct {
	ID            int
	Rank          int
	Title         string
	Band          string
	Country       string
	Year          int
	TimesListened int
	CoverURL      string
}

var db *sql.DB

func main() {
	rand.Seed(time.Now().UnixNano())

	var err error
	db, err = sql.Open("sqlite", "./albums.db")
	if err != nil {
		panic(err)
	}

	createTable()
	loadAlbumsFromCSV("data/albums.csv")
	fetchMissingCovers() // busca y guarda portadas secuencialmente

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	r.GET("/", func(c *gin.Context) {
		albums := getAllAlbums()
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
		id, _ := strconv.Atoi(c.Param("id"))
		album := markListened(id)
		if album == nil {
			c.JSON(404, gin.H{"error": "Álbum no encontrado"})
			return
		}
		c.JSON(200, album)
	})

	r.POST("/reset", func(c *gin.Context) {
		resetAlbums()
		c.JSON(200, gin.H{"message": "Lista reiniciada"})
	})

	r.Run(":8080")
}

// ---------------- DB ----------------

func createTable() {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS albums (
		id INTEGER PRIMARY KEY,
		rank INTEGER,
		title TEXT,
		band TEXT,
		country TEXT,
		year INTEGER,
		times_listened INTEGER,
		cover_url TEXT
	)`)
	if err != nil {
		panic(err)
	}
}

func loadAlbumsFromCSV(path string) {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		panic(err)
	}

	tx, _ := db.Begin()
	defer tx.Commit()

	for i, row := range rows {
		if i == 0 { // saltar cabecera
			continue
		}
		rank, _ := strconv.Atoi(row[0])
		year, _ := strconv.Atoi(row[4])
		if row[1] == "" || row[2] == "" {
			continue
		}

		_, err := tx.Exec(`
			INSERT OR IGNORE INTO albums (id, rank, title, band, country, year, times_listened, cover_url)
			VALUES (?, ?, ?, ?, ?, ?, 0, '')`,
			i, rank, row[1], row[2], row[3], year)
		if err != nil {
			fmt.Println("Error insertando:", row[1], row[2], err)
		}
	}
}

func getAllAlbums() []Album {
	rows, _ := db.Query("SELECT id, rank, title, band, country, year, times_listened, cover_url FROM albums ORDER BY rank")
	defer rows.Close()
	albums := []Album{}
	for rows.Next() {
		var a Album
		rows.Scan(&a.ID, &a.Rank, &a.Title, &a.Band, &a.Country, &a.Year, &a.TimesListened, &a.CoverURL)
		albums = append(albums, a)
	}
	return albums
}

func getRandomUnlistenedAlbum() *Album {
	albums := getAllAlbums()
	unlistened := []*Album{}
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

func markListened(id int) *Album {
	_, err := db.Exec("UPDATE albums SET times_listened = times_listened + 1 WHERE id = ?", id)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	row := db.QueryRow("SELECT id, rank, title, band, country, year, times_listened, cover_url FROM albums WHERE id = ?", id)
	var a Album
	row.Scan(&a.ID, &a.Rank, &a.Title, &a.Band, &a.Country, &a.Year, &a.TimesListened, &a.CoverURL)
	return &a
}

func resetAlbums() {
	_, err := db.Exec("UPDATE albums SET times_listened = 0")
	if err != nil {
		fmt.Println(err)
	}
}

// ---------------- Covers ----------------

// Normaliza caracteres especiales
func clean(s string) string {
	replacer := strings.NewReplacer(
		"®", "", // eliminar
		"™", "", // eliminar
		"♯", "sharp",
		"∞", "infinity",
		"&quot;", `"`,
		"&amp;", "&",
		"´", "'",
		"`", "'",
		"’", "'",
		"[", "", // opcional: eliminar corchetes
		"]", "",
		"(", "", // opcional: eliminar paréntesis
		")", "",
	)
	return replacer.Replace(s)
}

func fetchMissingCovers() {
	// Lanzamos en goroutine para no bloquear la app
	go func() {
		albums := getAllAlbums()
		for _, a := range albums {
			if a.CoverURL == "" {
				cleanTitle := clean(a.Title)
				cleanBand := clean(a.Band)
				cover := getAlbumCoverDeezer(cleanTitle, cleanBand)
				if cover != "" {
					_, err := db.Exec("UPDATE albums SET cover_url=? WHERE id=?", cover, a.ID)
					if err != nil {
						fmt.Println("Error guardando cover:", a.Title, a.Band, err)
					} else {
						fmt.Println("✅ Guardada portada:", a.Title, a.Band)
					}
				} else {
					fmt.Printf("⚠️ No se pudo obtener portada: %s - %s\n", a.Band, a.Title)
				}
				// Pausa pequeña para no saturar Deezer
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()
}

func getAlbumCoverDeezer(title, band string) string {
	title = clean(title)
	band = clean(band)

	query := url.QueryEscape(fmt.Sprintf("%s %s", band, title))
	apiURL := fmt.Sprintf("https://api.deezer.com/search/album?q=%s", query)

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error Deezer:", err)
		return ""
	}
	defer resp.Body.Close()

	if !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		fmt.Printf("⚠️ Deezer no devolvió JSON para: %s - %s\n", band, title)
		return ""
	}

	var data struct {
		Data []struct {
			Title       string                `json:"title"`
			Artist      struct{ Name string } `json:"artist"`
			CoverMedium string                `json:"cover_medium"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("Error decodificando Deezer para %s - %s: %v\n", band, title, err)
		return ""
	}

	t := strings.ToLower(title)
	b := strings.ToLower(band)

	for _, d := range data.Data {
		if strings.Contains(strings.ToLower(d.Title), t) &&
			strings.Contains(strings.ToLower(d.Artist.Name), b) {
			return d.CoverMedium
		}
	}

	if len(data.Data) > 0 {
		return data.Data[0].CoverMedium
	}

	return ""
}
