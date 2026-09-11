package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/gorilla/websocket"
)

func main() {
	token := os.Getenv("TOKEN")
	if token == "" {
		log.Fatal("TOKEN environment variable is empty")
	}

	url := "ws://127.0.0.1:8080/api/v1/bookings/0deaf0e4-c078-4f07-887f-b9d87b4263f5/chat/ws?token=" + token

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("websocket connection failed: ", err)
	}
	defer conn.Close()

	fmt.Println("WEBSOCKET_CONNECTED")
	fmt.Println("Type a message and press Enter:")

	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			fmt.Printf("\nRECEIVED: %s\n> ", message)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		message := scanner.Text()

		if err := conn.WriteJSON(map[string]string{
			"message": message,
		}); err != nil {
			log.Println("write failed:", err)
			return
		}

		fmt.Print("> ")
	}
}
