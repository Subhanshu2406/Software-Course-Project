package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// allowedContainers is a hardcoded allowlist of container names that can be managed.
var allowedContainers = map[string]bool{
	"shard1": true, "shard2": true, "shard3": true,
	"shard1a": true, "shard1b": true,
	"shard2a": true, "shard2b": true,
	"shard3a": true, "shard3b": true,
	"coordinator": true, "coordinator2": true,
	"load-monitor": true, "api-gateway": true,
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := envOrDefault("FAULT_PROXY_ADDR", ":8099")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("fault-proxy: cannot create Docker client: %v", err)
	}
	defer cli.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/kill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		name := r.URL.Query().Get("container")
		if !allowedContainers[name] {
			httpErr(w, http.StatusForbidden, "container not in allowlist")
			return
		}
		// Find container by name pattern (docker compose uses project prefix)
		containerID, err := findContainer(cli, name)
		if err != nil {
			httpErr(w, http.StatusNotFound, err.Error())
			return
		}
		timeout := 10
		if err := cli.ContainerStop(context.Background(), containerID, container.StopOptions{Timeout: &timeout}); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "stopped", "container": name})
	})

	mux.HandleFunc("/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		name := r.URL.Query().Get("container")
		if !allowedContainers[name] {
			httpErr(w, http.StatusForbidden, "container not in allowlist")
			return
		}
		containerID, err := findContainer(cli, name)
		if err != nil {
			httpErr(w, http.StatusNotFound, err.Error())
			return
		}
		if err := cli.ContainerStart(context.Background(), containerID, types.ContainerStartOptions{}); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "started", "container": name})
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("container")
		if !allowedContainers[name] {
			httpErr(w, http.StatusForbidden, "container not in allowlist")
			return
		}
		containerID, err := findContainer(cli, name)
		if err != nil {
			jsonOK(w, map[string]string{"status": "not_found", "container": name})
			return
		}
		info, err := cli.ContainerInspect(context.Background(), containerID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		state := "stopped"
		if info.State.Running {
			state = "running"
		}
		jsonOK(w, map[string]string{"status": state, "container": name})
	})

	log.Printf("fault-proxy listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("fault-proxy: %v", err)
	}
}

func findContainer(cli *client.Client, name string) (string, error) {
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return "", err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			// Docker prepends "/" to container names
			cleanName := strings.TrimPrefix(n, "/")
			if cleanName == name {
				return c.ID, nil
			}
			// Docker compose names include project prefix: "project-name-1"
			if strings.HasSuffix(cleanName, "-"+name) || strings.HasSuffix(cleanName, "-"+name+"-1") {
				return c.ID, nil
			}
		}
	}
	return "", fmt.Errorf("container %s not found", name)
}

func httpErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
