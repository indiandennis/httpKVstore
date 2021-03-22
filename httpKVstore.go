package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

var authkey string
var db *bolt.DB

func main() {
	nuCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(nuCPU)
	authkey = os.Getenv("AUTHKEY")
	log.Println(authkey)

	var err error
	db, err := bolt.Open("my.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("main"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})

	//gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/api/:authkey/:key", get)
	r.PUT("/api/:authkey/:key", set)

	srv := &http.Server{
		Addr:    ":8000",
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

	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("main"))
		v := b.Get([]byte(key))
		copy(value, v)
		return nil
	})

	if value == nil {
		c.Status(http.StatusNotFound)
		return
	}

	c.Data(http.StatusOK, "application/json", value)
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

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("main"))
		err := b.Put([]byte(key), newValue)
		return err
	})

	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}
