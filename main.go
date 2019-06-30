package main

import (
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-ini/ini"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

const configFile = "config.ini"

var (
	cfg *ini.Section
	// DB accès base de donnée sqlite3, au niveau de cette application API
	DB *gorm.DB
)

func init() {
	cfgFile, err := ini.InsensitiveLoad(configFile)
	if err != nil {
		log.Fatalln("Erreur accès du fichier config.ini")
	}
	cfg, _ = cfgFile.GetSection("")
	db, err := gorm.Open(cfg.Key("DB_DIALECT").String(), cfg.Key("DB_PARAMS").String())
	if err != nil {
		log.Fatalln("Erreur ouverture base de donnée", err.Error())
	}
	DB = db
}

// SetupRouter le routeur pour recevoir les requetes HTTP
func SetupRouter() *gin.Engine {
	router := gin.Default()
	// router.Use(cors.Default())
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	// config.AllowOrigins = []string{"*"}
	// config.AddAllowOrigins("http://localhost:8080")
	config.AllowHeaders = []string{"Origin", "Authorization", "Content-Type"}
	config.ExposeHeaders = []string{"Content-Length"}
	config.AllowCredentials = true
	config.MaxAge = 12 * time.Hour // - Preflight requests cached for 12 hours

	router.Use(cors.New(config))
	router.GET("/ping", GetPing)
	return router
}

// GetPing répond par un pong
func GetPing(c *gin.Context) {
	c.JSON(200, gin.H{"message": "pong"})
}

func main() {
	gin.SetMode(gin.DebugMode)
	defer DB.Close()
	router := SetupRouter()
	router.Run(cfg.Key("ADDR").String())
}
