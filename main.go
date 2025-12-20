// Package main implements the Minewire proxy server that masquerades as a Minecraft server.
// It accepts connections from Minewire clients and establishes encrypted tunnels for proxying traffic.
package main

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the server configuration loaded from server.yaml
type Config struct {
	ListenPort string   `yaml:"listen_port"`
	Passwords  []string `yaml:"passwords"` // List of authorized passwords

	// Minecraft server metadata for masquerading
	VersionName string `yaml:"version_name"`
	ProtocolID  int    `yaml:"protocol_id"`
	IconPath    string `yaml:"icon_path"`
	Motd        string `yaml:"motd"`

	// Player count simulation settings
	MaxPlayers int `yaml:"max_players"`
	OnlineMin  int `yaml:"online_min"`
	OnlineMax  int `yaml:"online_max"`
}

var cfg Config

func main() {
	f, err := os.Open("server.yaml")
	if err != nil {
		log.Fatal("Could not open server.yaml: ", err)
	}
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		log.Fatal("Invalid server.yaml: ", err)
	}
	f.Close()

	// Apply defaults if not specified in config
	if cfg.ProtocolID == 0 {
		cfg.ProtocolID = 773
	}
	if cfg.MaxPlayers == 0 {
		cfg.MaxPlayers = 20
	}

	// Initialize authentication map (convert passwords to expected usernames)
	initAuthMap()

	listener, err := net.Listen("tcp", "0.0.0.0:"+cfg.ListenPort)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Minewire Server started (version: %s, protocol: %d, port: %s)", cfg.VersionName, cfg.ProtocolID, cfg.ListenPort)

	// Start Player Count Simulator
	go startPlayerCountSimulator()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic: %v", r)
			conn.Close()
		}
	}()

	reader := bufio.NewReader(conn)
	state := 0

	for {
		length, err := ReadVarInt(reader)
		if err != nil {
			conn.Close()
			return
		}

		if length < 0 || length > 1048576 { // Sanity check
			conn.Close()
			return
		}

		packetData := make([]byte, length)
		_, err = io.ReadFull(reader, packetData)
		if err != nil {
			conn.Close()
			return
		}

		pBuf := bytes.NewBuffer(packetData)
		processPacket(conn, reader, pBuf, &state)
	}
}
