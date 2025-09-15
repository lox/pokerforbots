package main

import (
	"flag"
	"log"

	"github.com/lox/pokerforbots/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "Server address")
	flag.Parse()

	srv := server.NewServer()
	if err := srv.Start(*addr); err != nil {
		log.Fatal("Server error: ", err)
	}
}