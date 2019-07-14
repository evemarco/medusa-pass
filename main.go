package main

import (
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-ini/ini"
	"github.com/imroc/req"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

const configFile = "medusa-pass.ini"

var (
	cfg *ini.Section
	// DB accès base de donnée sqlite3, au niveau de cette application API
	DB *gorm.DB
	// header map[string]string
	b64 string
)

// Token table pour Gorm
type Token struct {
	gorm.Model
	AccessToken   string
	RefreshToken  string
	CharacterID   int64
	CharacterName string
}

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
	db.AutoMigrate(&Token{})
	DB = db
	b64 = base64.StdEncoding.EncodeToString([]byte(cfg.Key("CLIENT_ID").String() + ":" + cfg.Key("SECRET_KEY").String()))
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
	// /token=code=xxx
	router.GET("/token", GetToken)
	// /refresh { token: xxx, client_ID: xxx }
	router.POST("/refresh", PostRefresh)
	return router
}

// GetPing répond par un pong
func GetPing(c *gin.Context) {
	c.JSON(200, gin.H{"message": "pong"})
}

// GetToken répond par un token
func GetToken(c *gin.Context) {
	code, _ := c.GetQuery("code")
	header := req.Header{
		"Accept":        "application/json",
		"Authorization": "Basic " + b64,
		"User-Agent":    cfg.Key("USER_AGENT").String(),
	}
	param := req.Param{
		"grant_type": "authorization_code",
		"code":       code,
	}
	// only url is required, others are optional.
	r, err := req.Post("https://login.eveonline.com/oauth/token", header, param)
	if err != nil {
		log.Println("Erreur requête obtention token")
		c.JSON(500, gin.H{"status": http.StatusInternalServerError, "error": err})
		return
	}
	var result map[string]interface{}
	r.ToJSON(&result)    // response => struct/map
	log.Printf("%+v", r) // print info (try it, you may surprise)
	accessToken, _ := result["access_token"].(string)
	refreshToken, _ := result["refresh_token"].(string)
	header["Authorization"] = "Bearer " + accessToken
	r, err = req.Get("https://login.eveonline.com/oauth/verify", header)
	if err != nil {
		log.Println("Erreur requête vérification")
		c.JSON(500, gin.H{"status": http.StatusInternalServerError, "error": err})
		return
	}
	r.ToJSON(&result)
	log.Printf("%+v", r)
	characterName, _ := result["CharacterName"].(string)
	characterID, _ := result["CharacterID"].(float64)
	DB.Create(&Token{AccessToken: accessToken, RefreshToken: refreshToken, CharacterID: int64(characterID), CharacterName: characterName})
	c.JSON(200, gin.H{"token": accessToken, "characterName": characterName, "characterID": characterID})
}

// Refresh to map post request
type Refresh struct {
	Token    string `form:"token" json:"token" xml:"token"  binding:"required"`
	ClientID string `form:"client_ID" json:"client_ID" xml:"client_ID" binding:"required"`
}

// PostRefresh répond par un nouveau token par le refresh token stocké en base de donnée
func PostRefresh(c *gin.Context) {
	var refresh Refresh
	if c.ShouldBind(&refresh) != nil {
		if refresh.Token == "" {
			log.Println("Token manquant")
			c.JSON(500, gin.H{"status": http.StatusInternalServerError, "error": "Token not found"})
			return
		}
		if refresh.ClientID == "" || (refresh.ClientID != cfg.Key("CLIENT_ID").String()) {
			log.Println("Client_id manquant ou non correspondant")
			c.JSON(500, gin.H{"status": http.StatusInternalServerError, "error": "Client_id not found"})
			return
		}
	}
	var t Token
	DB.Where(&Token{AccessToken: refresh.Token}).First(&t)
	if t.RefreshToken == "" {
		log.Println("AccessToken non trouvé en BDD")
		c.JSON(500, gin.H{"status": http.StatusInternalServerError, "error": "No access token found"})
		return
	}
	header := req.Header{
		"Accept":        "application/json",
		"Authorization": "Basic " + b64,
		"User-Agent":    cfg.Key("USER_AGENT").String(),
	}
	param := req.Param{
		"grant_type":    "refresh_token",
		"refresh_token": t.RefreshToken,
	}
	r, err := req.Post("https://login.eveonline.com/oauth/token", header, param)
	if err != nil {
		log.Println("Erreur requête obtention access_token par le refresh_token")
		c.JSON(500, gin.H{"status": http.StatusInternalServerError, "error": err})
		return
	}
	var result map[string]interface{}
	r.ToJSON(&result) // response => struct/map
	log.Println(r)
	accessToken, _ := result["access_token"].(string)
	t.AccessToken = accessToken
	DB.Save(&t)
	c.JSON(200, gin.H{"token": accessToken})
}

func main() {
	gin.SetMode(gin.DebugMode)
	defer DB.Close()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("\r- Ctrl+C pressed in Terminal")
		// cleanup()
		os.Exit(1)
	}()
	router := SetupRouter()
	router.Run(cfg.Key("ADDR").String())
}
