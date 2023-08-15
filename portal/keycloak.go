package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

var keycloakImage = flag.String("keycloak_image", "naive.systems/box/keycloak:dev", "")

var keycloakCmd *exec.Cmd

func StartKeycloak() {
	keycloakDir := filepath.Join(*workdir, "keycloak")
	err := os.MkdirAll(keycloakDir, 0700)
	if err != nil {
		log.Fatalf("os.MkdirAll(%s): %v", keycloakDir, err)
	}
	PodmanKillKeycloak()
	versionFile := filepath.Join(keycloakDir, "version.txt")
	if exists(versionFile) {
		RunKeycloak()
	} else {
		InstallAndRunKeycloak()
	}
}

func InstallAndRunKeycloak() {
	ExtractKeycloak()

	// Let it initialize its database
	RunKeycloak()
	WaitKeycloakUp()
	StopKeycloak()

	time.Sleep(1 * time.Second)

	RunKeycloak()
	WaitKeycloakUp()
	InitKeycloak()
}

func GetKeycloakStatus() (string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Get("https://127.0.0.1:9992/health/ready")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var status struct {
		Status string `json:"status"`
	}
	err = json.Unmarshal(body, &status)
	if err != nil {
		return "", err
	}

	return status.Status, nil
}

func WaitKeycloakUp() {
	for {
		time.Sleep(1 * time.Second)
		status, err := GetKeycloakStatus()
		if status == "UP" {
			break
		}
		if err == nil {
			log.Printf("Keycloak is not up: %s", status)
		} else {
			log.Printf("Keycloak is not up: %v", err)
		}
	}
	log.Println("Keycloak is up")
}

func ExtractKeycloak() {
	keycloakDir := filepath.Join(*workdir, "keycloak")
	cmd := exec.Command("podman", "run", "--rm",
		"--name", "keycloak", "--replace",
		"--userns=keep-id:uid=1000,gid=1000",
		"-v", keycloakDir+":/home/keycloak/keycloak",
		*keycloakImage,
		"/home/keycloak/extract")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to extract Keycloak: %v", err)
	}
}

func InitKeycloak() {
	cmd := exec.Command("podman", "exec", "keycloak", "/home/keycloak/init")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to initialize Keycloak: %v", err)
	}
}

func RunKeycloak() {
	certsDir := filepath.Join(*workdir, "certs")
	keycloakDir := filepath.Join(*workdir, "keycloak")
	keycloakCmd = exec.Command("podman", "run", "--rm",
		"--name", "keycloak", "--replace",
		"--userns=keep-id:uid=1000,gid=1000",
		"-v", certsDir+":/certs",
		"-v", keycloakDir+":/home/keycloak/keycloak",
		"-p", "9992:9992/tcp",
		"--add-host", *hostname+":127.0.0.1",
		*keycloakImage,
		"/home/keycloak/run", "--hostname", *hostname)
	keycloakCmd.Stdout = os.Stdout
	keycloakCmd.Stderr = os.Stderr
	err := keycloakCmd.Start()
	if err != nil {
		log.Fatalf("Failed to start Keycloak: %v", err)
	}
}

func StopKeycloak() {
	err := keycloakCmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		log.Fatalf("Failed to stop Keycloak: %v", err)
	}
	PodmanKillKeycloak()
}

func PodmanKillKeycloak() {
	cmd := exec.Command("podman", "kill", "keycloak")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Printf("podman kill keycloak: %v", err)
	}
}
