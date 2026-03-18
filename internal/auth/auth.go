package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"

	"golang.org/x/term"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func Login(instanceURL, username, password string) (string, error) {
	url := strings.TrimRight(instanceURL, "/") + "/api/auth/login"

	body, _ := json.Marshal(loginRequest{
		Username: username,
		Password: password,
	})

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	var result loginResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("unexpected response from server")
	}

	if !result.Success || result.Token == "" {
		msg := "authentication failed"
		if result.Error != nil {
			msg = result.Error.Message
		}
		return "", fmt.Errorf(msg)
	}

	return result.Token, nil
}

func PromptCredentials() (string, string, error) {
	fmt.Print("Username: ")
	var username string
	if _, err := fmt.Scanln(&username); err != nil {
		return "", "", err
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", "", err
	}
	fmt.Println()

	return username, string(passwordBytes), nil
}
