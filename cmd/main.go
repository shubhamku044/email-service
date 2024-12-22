package main

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gopkg.in/gomail.v2"
)

type ContactForm struct {
	Email   string `form:"email" binding:"required"`
	Message string `form:"message" binding:"required"`
	Name    string `form:"name" binding:"required"`
}

type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
	}
}

// Allow checks if the request should be allowed based on IP
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-24 * time.Hour) // 24-hour window

	// Clean old requests
	if times, exists := rl.requests[ip]; exists {
		var valid []time.Time
		for _, t := range times {
			if t.After(windowStart) {
				valid = append(valid, t)
			}
		}
		rl.requests[ip] = valid
	}

	// Check rate limit (5 requests per 24 hours)
	if len(rl.requests[ip]) >= 5 {
		return false
	}

	// Add new request
	rl.requests[ip] = append(rl.requests[ip], now)
	return true
}

var rateLimiter = NewRateLimiter()

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://localhost:3000", "https://shubhams.dev", "https://www.shubhams.dev"}
	config.AllowMethods = []string{"POST"}
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type"}

	r.Use(cors.New(config))

	r.SetTrustedProxies([]string{"127.0.0.1"})

	r.POST("/api/contact", rateLimit, handleContact)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r.Run(":" + port)
}

func rateLimit(c *gin.Context) {
	ip := c.ClientIP()
	if !rateLimiter.Allow(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Rate limit exceeded. Please try again after 24 hours.",
		})
		c.Abort()
		return
	}
	c.Next()
}

func handleContact(c *gin.Context) {
	var form ContactForm
	if err := c.ShouldBind(&form); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	m := gomail.NewMessage()
	m.SetHeader("From", os.Getenv("EMAIL_FROM"))
	m.SetHeader("To", os.Getenv("EMAIL_TO"))
	m.SetHeader("Subject", "Contact Form Submission "+form.Name)
	m.SetBody("text/plain", "Name: "+form.Name+"\nEmail: "+form.Email+"\nMessage: "+form.Message)

	d := gomail.NewDialer("smtp.gmail.com", 587, os.Getenv("EMAIL_FROM"), os.Getenv("EMAIL_APP_PASSWORD"))

	if err := d.DialAndSend(m); err != nil {
		log.Printf("Failed to send email: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email sent"})
}
