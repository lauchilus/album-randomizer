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
	"os/exec"
	"runtime"
	"strconv"
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

	// Abrir navegador automáticamente
	go func() {
		url := "http://localhost:8080"
		switch runtime.GOOS {
		case "windows":
			exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		case "darwin":
			exec.Command("open", url).Start()
		case "linux":
			exec.Command("xdg-open", url).Start()
		}
	}()

	// Iniciar servidor en paralelo a la carga de portadas
	go fetchMissingCovers() // carga secuencial sin bloquear la app

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

func fetchMissingCovers() {
	albums := getAllAlbums()
	for _, a := range albums {
		if a.CoverURL == "" {
			cover := getAlbumCoverDeezer(a.Title, a.Band)
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
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func getAlbumCoverDeezer(title, band string) string {
	// reemplaza caracteres problemáticos para que la query funcione
	clean := func(s string) string {
		replacer := []string{
			"♯", "#",
			"∞", "infinity",
			"®", "",
			"’", "'",
			"“", "\"",
			"”", "\"",
			"&quot;", "\"",
		}
		for i := 0; i < len(replacer); i += 2 {
			s = replaceAll(s, replacer[i], replacer[i+1])
		}
		return s
	}

	query := url.QueryEscape(fmt.Sprintf("%s %s", clean(band), clean(title)))
	apiURL := fmt.Sprintf("https://api.deezer.com/search/album?q=%s", query)

	client := &http.Client{}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error Deezer:", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "application/json" {
		fmt.Printf("⚠️ Deezer no devolvió JSON para: %s - %s\n", band, title)
		return ""
	}

	var data struct {
		Data []struct {
			CoverMedium string `json:"cover_medium"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Println("Error decodificando Deezer:", err)
		return ""
	}

	if len(data.Data) > 0 {
		return data.Data[0].CoverMedium
	}

	return ""
}

func replaceAll(s, old, new string) string {
	return string([]rune(s))
}
