package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"inkwell-backend-V2.0/internal/config"
	"inkwell-backend-V2.0/internal/db"
	"inkwell-backend-V2.0/internal/model"
	"inkwell-backend-V2.0/internal/repository"
	"inkwell-backend-V2.0/internal/service"
)

func main() {
	printStartUpBanner()

	// Load XML configuration from file.
	cfg, err := config.LoadConfig("config.xml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize DB using the loaded config.
	db.InitDBFromConfig(cfg)
	// Run migrations.
	db.GetDB().AutoMigrate(&model.User{}, &model.Assessment{}, &model.Story{})

	// Create repositories.
	userRepo := repository.NewUserRepository()
	assessmentRepo := repository.NewAssessmentRepository()
	storyRepo := repository.NewStoryRepository()

	// Create services.
	authService := service.NewAuthService(userRepo)
	userService := service.NewUserService(userRepo)
	assessmentService := service.NewAssessmentService(assessmentRepo)
	storyService := service.NewStoryService(storyRepo)

	// Initialize Gin router.
	r := gin.Default()

	// CORS configuration.
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Auth routes.
	auth := r.Group("/auth")
	{
		auth.POST("/register", func(c *gin.Context) {
			var user model.User
			if err := c.ShouldBindJSON(&user); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
				return
			}
			if err := authService.Register(&user); err != nil {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
		})

		auth.POST("/login", func(c *gin.Context) {
			var creds struct {
				Email    string `json:"email"`
				AuthHash string `json:"authhash"`
			}
			if err := c.ShouldBindJSON(&creds); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
				return
			}
			user, err := authService.Login(creds.Email, creds.AuthHash)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, user)
		})
	}

	// User routes.
	r.GET("/user", func(c *gin.Context) {
		users, err := userService.GetAllUsers()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, users)
	})

	// Assessment routes.
	assessmentRoutes := r.Group("/assessments")
	{
		// Start an assessment
		assessmentRoutes.POST("/start", func(c *gin.Context) {
			var req struct {
				UserID      uint             `json:"user_id"`
				Title       string           `json:"title"`
				Description string           `json:"description"`
				Questions   []model.Question `json:"questions"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
				return
			}

			assessment, err := assessmentService.CreateAssessment(req.UserID, req.Title, req.Description, req.Questions)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"session_id": assessment.SessionID,
				"questions":  assessment.Questions,
			})
		})

		// Submit an answer
		assessmentRoutes.POST("/submit", func(c *gin.Context) {
			var req struct {
				SessionID  string `json:"session_id"`
				QuestionID uint   `json:"question_id"`
				Answer     string `json:"answer"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
				return
			}

			assessment, err := assessmentService.GetAssessmentBySessionID(req.SessionID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
				return
			}

			var question model.Question
			for _, q := range assessment.Questions {
				if q.ID == req.QuestionID {
					question = q
					break
				}
			}

			if question.ID == 0 {
				c.JSON(http.StatusNotFound, gin.H{"error": "Question not found"})
				return
			}

			isCorrect := question.CorrectAnswer == req.Answer
			feedback := "Incorrect"
			if isCorrect {
				feedback = "Correct"
			}

			answer := model.Answer{
				AssessmentID: assessment.ID,
				QuestionID:   req.QuestionID,
				UserID:       assessment.UserID,
				Answer:       req.Answer,
				IsCorrect:    isCorrect,
				Feedback:     feedback,
			}

			if err := assessmentService.SaveAnswer(&answer); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"is_correct": isCorrect,
				"feedback":   feedback,
			})
		})

		// Get a specific assessment
		assessmentRoutes.GET("/:session_id", func(c *gin.Context) {
			sessionID := c.Param("session_id")

			assessment, err := assessmentService.GetAssessmentBySessionID(sessionID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Assessment not found"})
				return
			}
			c.JSON(http.StatusOK, assessment)
		})
	}

	// Story routes.
	r.GET("/stories", func(c *gin.Context) {
		stories, err := storyService.GetStories()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, stories)
	})

	// Start server on the host and port specified in the XML config.
	addr := fmt.Sprintf("%s:%d", cfg.Context.Host, cfg.Context.Port)
	r.Run(addr)
}

func printStartUpBanner() {
	myFigure := figure.NewFigure("INKWELL", "", true)
	myFigure.Print()

	fmt.Println("======================================================")
	fmt.Printf("INKWELL API (v%s)\n\n", "2.0.0-StoryScape")
}
