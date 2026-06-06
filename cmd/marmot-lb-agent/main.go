package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type agentState struct {
	LastAppliedHash string    `json:"lastAppliedHash"`
	LastAppliedAt   time.Time `json:"lastAppliedAt"`
	LastError       string    `json:"lastError,omitempty"`
}

func main() {
	intervalSeconds := flag.Int("interval-seconds", 5, "Reconcile interval in seconds")
	desiredConfig := flag.String("desired-config", "/etc/haproxy/haproxy-desired.cfg", "Path to desired HAProxy config")
	activeConfig := flag.String("active-config", "/etc/haproxy/haproxy.cfg", "Path to active HAProxy config")
	stateFile := flag.String("state-file", "/var/lib/marmot/lb-agent/state.json", "Path to agent state file")
	lockFile := flag.String("lock-file", "/var/lib/marmot/lb-agent/agent.lock", "Path to single-instance lock file")
	flag.Parse()

	if *intervalSeconds <= 0 {
		log.Fatalf("interval-seconds must be positive")
	}

	lockHandle, err := acquireLock(*lockFile)
	if err != nil {
		log.Fatalf("failed to acquire lock: %v", err)
	}
	defer releaseLock(lockHandle)

	ticker := time.NewTicker(time.Duration(*intervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		reconcile(*desiredConfig, *activeConfig, *stateFile)
		<-ticker.C
	}
}

func acquireLock(path string) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("lock file path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func releaseLock(file *os.File) {
	if file == nil {
		return
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}

func reconcile(desiredConfig, activeConfig, stateFile string) {
	desired, err := os.ReadFile(desiredConfig)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("desired config not found yet: %s", desiredConfig)
			return
		}
		log.Printf("failed to read desired config: %v", err)
		return
	}

	hash := sha256.Sum256(desired)
	desiredHash := hex.EncodeToString(hash[:])

	state := loadState(stateFile)
	if strings.TrimSpace(state.LastAppliedHash) == desiredHash {
		return
	}

	tmpPath := activeConfig + ".candidate"
	if err := os.WriteFile(tmpPath, desired, 0o644); err != nil {
		state.LastError = fmt.Sprintf("failed to write candidate config: %v", err)
		saveState(stateFile, state)
		log.Printf("%s", state.LastError)
		return
	}

	if output, err := exec.Command("haproxy", "-c", "-f", tmpPath).CombinedOutput(); err != nil {
		state.LastError = fmt.Sprintf("haproxy validation failed: %v: %s", err, strings.TrimSpace(string(output)))
		saveState(stateFile, state)
		log.Printf("%s", state.LastError)
		_ = os.Remove(tmpPath)
		return
	}

	if err := os.Rename(tmpPath, activeConfig); err != nil {
		state.LastError = fmt.Sprintf("failed to activate config: %v", err)
		saveState(stateFile, state)
		log.Printf("%s", state.LastError)
		return
	}

	if output, err := exec.Command("systemctl", "reload", "haproxy").CombinedOutput(); err != nil {
		state.LastError = fmt.Sprintf("haproxy reload failed: %v: %s", err, strings.TrimSpace(string(output)))
		saveState(stateFile, state)
		log.Printf("%s", state.LastError)
		return
	}

	state.LastAppliedHash = desiredHash
	state.LastAppliedAt = time.Now()
	state.LastError = ""
	saveState(stateFile, state)
	log.Printf("applied desired haproxy config: hash=%s", desiredHash)
}

func loadState(path string) agentState {
	data, err := os.ReadFile(path)
	if err != nil {
		return agentState{}
	}
	var state agentState
	if err := json.Unmarshal(data, &state); err != nil {
		return agentState{}
	}
	return state
}

func saveState(path string, state agentState) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("failed to create state directory: %v", err)
		return
	}
	data, err := json.Marshal(state)
	if err != nil {
		log.Printf("failed to marshal state: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("failed to write state: %v", err)
	}
}