package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/websocket/v2"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nedpals/supabase-go"
)

var db *sql.DB

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
	err := godotenv.Load()
	if err != nil {
		log.Print("Error loading .env file: %v", err)
	}

	// dbconn := os.Getenv("DB_CONNECTION_STRING")
	// db, err := sql.Open("postgres", dbconn)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer db.Close()

	// // Run migration script to create table
	// err = migrateDB(db)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// Supabase conn
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")
	client := supabase.CreateClient(supabaseURL, supabaseKey)

	app := fiber.New()
	app.Use(cors.New())

	// routes
	app.Post("/send-message", SendMessageHandler(client))
	app.Get("/fetch-messages", FetchMessagesHandler(client))
	app.Get("/ws", WebSocketHandler())

	port := os.Getenv("PORT")
	fmt.Println("Server is running on", port)
	log.Fatal(app.Listen(":" + port))
}

// SendMessageHandler send Message to supabase
func SendMessageHandler(client *supabase.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {

		var res []Message
		var msg Message
		if err := c.BodyParser(&msg); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		// Validate required fields
		if msg.SenderID == 0 || msg.ReceiverID == 0 || msg.Message == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "All fields must be provided"})
		}

		// Insert into SQLite database if needed
		// _, e := db.Exec(`
		// 	INSERT INTO messages (sender_id, receiver_id, message, timestamp)
		// 	VALUES (?, ?, ?, ?)`,
		// 	msg.SenderID, msg.ReceiverID, msg.Message, msg.Timestamp)

		// if e != nil {
		// 	log.Println("Failed to insert message into SQLite:", e)
		// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to insert message"})
		// }

		err := client.DB.From("messages").Insert([]map[string]interface{}{
			{
				"sender_id":   msg.SenderID,
				"receiver_id": msg.ReceiverID,
				"message":     msg.Message,
				"timestamp":   time.Now(),
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

func migrateDB(db *sql.DB) error {
	// Create table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS messages (
		id SERIAL PRIMARY KEY,
		sender_id INTEGER NOT NULL,
		receiver_id INTEGER NOT NULL,
		message TEXT NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(createTableSQL)
	if err != nil {
		return err
	}

	return nil
}
