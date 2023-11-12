package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/websocket/v2"
	"github.com/nedpals/supabase-go"
)

type Message struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SenderID   uint      `json:"sender_id"`
	ReceiverID uint      `json:"receiver_id"`
	Message    string    `json:"message"`
	Timestamp  time.Time `json:"timestamp"`
}

// var upgrader = websocket.Upgrader{
// 	ReadBufferSize:  1024,
// 	WriteBufferSize: 1024,
// 	CheckOrigin:     func(r *http.Request) bool { return true },
//  }

var clients = make(map[*websocket.Conn]bool)

func main() {
	// Supabase conn
	supabaseURL := "https://bhddowrlsurohkzttrhw.supabase.co"
	supabaseKey := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImJoZGRvd3Jsc3Vyb2hrenR0cmh3Iiwicm9sZSI6InNlcnZpY2Vfcm9sZSIsImlhdCI6MTY5OTYwNjM5MiwiZXhwIjoyMDE1MTgyMzkyfQ.8x4O5u9gh0veGM4CNoO-RxK_vjFbHTf-d6mjL1n12wk"
	client := supabase.CreateClient(supabaseURL, supabaseKey)

	app := fiber.New()
	app.Use(cors.New())

	// routes
	app.Post("/send-message", SendMessageHandler(client))
	app.Get("/fetch-messages", FetchMessagesHandler(client))
	app.Get("/ws", WebSocketHandler())

	fmt.Println("Server is running on :3000")
	log.Fatal(app.Listen(":3000"))
}

// SendMessageHandler send Message to supabase
func SendMessageHandler(client *supabase.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {

		var res []Message
		var msg Message
		if err := c.BodyParser(&msg); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}
		err := client.DB.From("messages").Insert([]map[string]interface{}{
			{
				"sender_id":   msg.SenderID,
				"receiver_id": msg.ReceiverID,
				"message":     msg.Message,
				"timestamp":   msg.Timestamp,
			},
		}).Execute(&res)
		if err != nil {
			log.Println("Failed to insert message into Supabase:", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to insert message"})
		}

		// Broadcast the new message to all connected WebSocket clients
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Println("Error broadcasting message:", err)
				delete(clients, client)
				client.Close()
			}
		}

		fmt.Println(res)
		return c.JSON(res)
	}
}

// FetchMessagesHandler fetch messages from supabase
func FetchMessagesHandler(client *supabase.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {

		var res []Message
		err := client.DB.From("messages").Select("*").Execute(&res)
		if err != nil {
			return err
		}
		fmt.Println(res)

		// Broadcast the new messages to all connected clients
		for client := range clients {
			err := client.WriteJSON(res)
			if err != nil {
				log.Println("Error broadcasting message:", err)
				delete(clients, client)
				client.Close()
			}
		}

		return c.JSON(res)
	}
}

// WebSocketHandler
func WebSocketHandler() fiber.Handler {
	return websocket.New(func(c *websocket.Conn) {
		clients[c] = true
		defer func() {
			delete(clients, c)
			c.Close()
		}()

		var mt int
		var msg []byte
		var err error
		for {
			if mt, msg, err = c.ReadMessage(); err != nil {
				log.Println("read:", err)
				break
			}
			log.Printf("recv: %s", msg)

			if err = c.WriteMessage(mt, msg); err != nil {
				log.Println("write:", err)
				break
			}
		}
	})
}
