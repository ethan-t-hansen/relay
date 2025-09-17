package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type User struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
}

type Component struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Desc      string `json:"description"`
	UpdatedAt string `json:"updated_at"`
}

type Library struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	FileKey             string      `json:"file_key"`
	PublishedComponents []Component `json:"published_components"`
}

type FigmaWebhook struct {
	EventType   string `json:"event_type"`
	FileKey     string `json:"file_key"`
	Timestamp   string `json:"timestamp"`
	TriggeredBy string `json:"triggered_by"`
	Webhooks    []struct {
		ID       string `json:"id"`
		TeamID   string `json:"team_id"`
		Endpoint string `json:"endpoint"`
	} `json:"webhooks"`
}

type LinearIssueInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	TeamID      string `json:"teamId"`
}

type LinearIssueRequest struct {
	Input LinearIssueInput `json:"input"`
}

func buildCreateIssueReqBody(title, description, teamId string) ([]byte, error) {
	query := `
        mutation IssueCreate($input: IssueCreateInput!) {
            issueCreate(input: $input) {
                issue {
                    id
                    title
                }
            }
        }
    `

	vars := map[string]interface{}{
		"input": map[string]string{
			"title":       title,
			"description": description,
			"teamId":      teamId,
		},
	}

	reqBody := GraphQLRequest{
		Query:     query,
		Variables: vars,
	}

	return json.Marshal(reqBody)
}

func createLinearIssue(title, description string) error {

	var linearToken = os.Getenv("LINEAR_API_KEY")
	var linearTeamID = os.Getenv("LINEAR_TEAM_ID")

	if linearToken == "" || linearTeamID == "" {
		return fmt.Errorf("missing LINEAR_API_KEY or LINEAR_TEAM_ID in env")
	}

	b, err := buildCreateIssueReqBody(title, description, linearTeamID)

	req, err := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", linearToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create issue, status: %s, body: %s", resp.Status, string(body))
	}

	log.Printf("Created Linear issue:", resp.Body)
	return nil

}

func createIssueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var webhook FigmaWebhook
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := json.Unmarshal(body, &webhook); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("Received Figma webhook: %+v", webhook)

	if webhook.EventType == "LIBRARY_PUBLISH" {
		title := fmt.Sprintf("Figma Library Published: %s", webhook.FileKey)
		description := fmt.Sprintf("The Figma file with key %s has published a new library at %s.", webhook.FileKey, webhook.Timestamp)

		if err := createLinearIssue(title, description); err != nil {
			http.Error(w, "Failed to create Linear issue: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Linear issue created successfully"))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Event type not handled"))
	}
}

func init() {
	_ = godotenv.Load()
}

func main() {

	http.HandleFunc("/create-issue", createIssueHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	log.Printf("Server starting on port %s", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
