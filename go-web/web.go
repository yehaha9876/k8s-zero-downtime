package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
        "syscall"
	"time"
)

func sayhelloName(w http.ResponseWriter, r *http.Request) {
	t := time.Now().Unix()
	log.Print("start ", t)
	time.Sleep(time.Duration(5) * time.Second)
	fmt.Fprintf(w, "Hello World!")
	log.Print("return ", t)
}

func readinessProbe(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Reday!")
}

func main() {
	time.Sleep(time.Duration(10) * time.Second)
	log.Print("Start server")
        var srv = &http.Server{Addr: ":9090"}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool)
        signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	http.HandleFunc("/", sayhelloName)
	http.HandleFunc("/ready", readinessProbe)
	go func() {
		<-sigs
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10) * time.Second)
		defer cancel()
                #time.Sleep(time.Duration(2) * time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			log.Println("Pre shutdown whit no sigl:", err)
                }
		close(done)
                log.Println("Graceful shutdown down ")
	}()

	err := srv.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Fatal("ListenAndServe: ", err)
	}
	<-done
}
