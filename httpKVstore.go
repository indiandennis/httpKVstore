package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

var authkey string
var db *badger.DB

func main() {
	nuCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(nuCPU)
	authkey = os.Getenv("authkey")

	var err error
	db, err = badger.Open(badger.DefaultOptions("./badger"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	//gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/api/:authkey/:key", get)
	r.PUT("/api/:authkey/:key", set)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be catch, so don't need add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func auth(c *gin.Context) bool {

	if c.Param("authkey") == authkey {
		return true
	}
	log.Println("Auth failed")
	c.AbortWithStatus(http.StatusUnauthorized)
	return false
}

func get(c *gin.Context) {
	if !auth(c) {
		return
	}
	key := c.Param("key")
	if key == "" {
		c.Status(http.StatusNotFound)
		return
	}
	var value []byte

	err := db.View((func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		value, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return nil
	}))

	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	c.JSON(http.StatusOK, string(value))
}

func set(c *gin.Context) {
	if !auth(c) {
		return
	}
	key := c.Param("key")
	newValue, err := c.GetRawData()
	if key == "" || err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	log.Println("Newval: ", newValue)

	err = db.Update(func(txn *badger.Txn) error {
		err := txn.Set([]byte(key), []byte(newValue))
		return err
	})

	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}
