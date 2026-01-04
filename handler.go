// Package main implements the Minewire proxy server.
// This file contains the protocol handlers and connection management logic.
package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

// Minecraft protocol packet IDs
const (
	PID_CB_StatusResp      = 0x00 // Server -> Client: Status response
	PID_CB_Ping            = 0x01 // Server -> Client: Ping
	PID_CB_LoginSuccess    = 0x02 // Server -> Client: Login success
	PID_CB_LoginDisconnect = 0x00 // Server -> Client: Disconnect during login
	PID_CB_JoinGame        = 0x29 // Server -> Client: Join game
	PID_CB_KeepAlive       = 0x24 // Server -> Client: Keep alive
	PID_CB_ChunkData       = 0x25 // Server -> Client: Chunk data
	PID_CB_PlayerPos       = 0x3E // Server -> Client: Synchronize Player Position
	PID_CB_TimeUpdate      = 0x62 // Server -> Client: Time Update

	PID_SB_PluginMsg = 0x0D // Client -> Server: Plugin message
)

// Global state for player count simulation and authentication
var (
	currentOnline int
	onlineLock    sync.Mutex
	validUsers    = make(map[string]string) // Map: GeneratedUsername -> OriginalPassword
	nicknameMap   = make(map[string]string) // Map: Nickname -> OriginalPassword
)

// initAuthMap initializes the authentication map by generating expected usernames
// from configured passwords. Clients generate usernames using the same algorithm.
func initAuthMap() {
	hashAndRegister := func(pwd, nick string) {
		h := sha256.Sum256([]byte(pwd))
		// Generate expected username the same way the client does
		expectedUser := "Player" + hex.EncodeToString(h[:])[:8]
		validUsers[expectedUser] = pwd
		if nick != "" {
			nicknameMap[nick] = pwd
			log.Printf("Registered agent access for: %s (Nick: %s)", expectedUser, nick)
		} else {
			log.Printf("Registered agent access for: %s", expectedUser)
		}
	}

	for _, item := range cfg.Passwords {
		switch v := item.(type) {
		case string:
			hashAndRegister(v, "")
		case map[string]interface{}:
			for pwd, nickVal := range v {
				if nick, ok := nickVal.(string); ok {
					hashAndRegister(pwd, nick)
				}
			}
		}
	}
}

// startPlayerCountSimulator simulates realistic player count fluctuations
// to make the server appear more legitimate when queried.
func startPlayerCountSimulator() {
	// Initialize with average player count
	onlineLock.Lock()
	currentOnline = (cfg.OnlineMin + cfg.OnlineMax) / 2
	onlineLock.Unlock()

	// Update player count every 30 minutes
	ticker := time.NewTicker(30 * time.Minute)
	for range ticker.C {
		onlineLock.Lock()
		// Apply smooth random change (-3 to +3 players)
		change := getSecureRandomInt(7) - 3
		newVal := currentOnline + change

		// Clamp to configured min/max range
		if newVal < cfg.OnlineMin {
			newVal = cfg.OnlineMin
		}
		if newVal > cfg.OnlineMax {
			newVal = cfg.OnlineMax
		}

		currentOnline = newVal
		log.Printf("Player count simulation: %d players online", currentOnline)
		onlineLock.Unlock()
	}
}

func getSecureRandomInt(max int) int {
	b := make([]byte, 1)
	rand.Read(b)
	return int(b[0]) % max
}

func processPacket(conn net.Conn, reader io.Reader, pBuf *bytes.Buffer, state *int) {
	pid, _ := ReadVarInt(pBuf)

	switch *state {
	case 0: // Handshake
		if pid == 0x00 {
			ReadVarInt(pBuf)
			l, _ := ReadVarInt(pBuf)
			pBuf.Next(l)
			pBuf.Next(2)
			*state, _ = ReadVarInt(pBuf)
		}
	case 1: // Status
		if pid == 0x00 {
			sendFakeStatus(conn)
		}
		if pid == 0x01 {
			WritePacket(conn, PID_CB_Ping, pBuf.Bytes())
		}
	case 2: // Login
		if pid == 0x00 {
			l, _ := ReadVarInt(pBuf)
			nameBytes := make([]byte, l)
			pBuf.Read(nameBytes)
			username := string(nameBytes)

			// Check if username is in the authorized users map
			if userPassword, ok := validUsers[username]; ok {
				log.Printf("Authorized agent connected: %s", username)
				// Pass the user's specific password for encryption key generation
				startDeepCoverSession(conn, username, reader, userPassword)
				return
			} else {
				log.Printf("Rejected unauthorized connection from: %s", username)
				sendDisconnect(conn, "§cNot whitelisted!")
				conn.Close()
				return
			}
		}
	}
}

// startDeepCoverSession establishes an encrypted tunnel session disguised as a Minecraft connection.
// It sends the necessary Minecraft protocol packets and then starts the multiplexed tunnel.
func startDeepCoverSession(conn net.Conn, username string, leftoverReader io.Reader, password string) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
		tcpConn.SetKeepAlive(true)
	}
	// Step 1: Send Login Success packet
	uuid := make([]byte, 16)
	rand.Read(uuid)
	buf := new(bytes.Buffer)
	buf.Write(uuid)
	WriteString(buf, username)
	WriteVarInt(buf, 0)
	WritePacket(conn, PID_CB_LoginSuccess, buf.Bytes())

	// Step 2: Send Join Game packet (Protocol 773 / Minecraft 1.21.10)
	buf.Reset()
	WriteInt(buf, 100)
	WriteBool(buf, false)
	WriteVarInt(buf, 1)
	WriteString(buf, "minecraft:overworld")
	WriteVarInt(buf, 0)
	WriteVarInt(buf, 8)
	WriteVarInt(buf, 8)
	WriteBool(buf, false)
	WriteBool(buf, true)
	WriteBool(buf, false)
	WriteVarInt(buf, 0)
	WriteString(buf, "minecraft:overworld")
	WriteLong(buf, 123456789)
	WriteByte(buf, 1)
	WriteByte(buf, 0xFF)
	WriteBool(buf, false)
	WriteBool(buf, false)
	WriteBool(buf, false)
	WriteVarInt(buf, 0)
	WriteVarInt(buf, 63)
	WriteBool(buf, false)
	WritePacket(conn, PID_CB_JoinGame, buf.Bytes())

	// Step 3: Send Synchronize Player Position (Protocol 773 / 1.20.4-1.21.x mix)
	// Sets the initial player position to a realistic value
	motion := NewMotionGenerator()
	buf.Reset()
	WriteDouble(buf, motion.X)
	WriteDouble(buf, motion.Y)
	WriteDouble(buf, motion.Z)
	WriteFloat(buf, float32(motion.Angle*180/math.Pi)) // Yaw
	WriteFloat(buf, 0)                                 // Pitch
	WriteByte(buf, 0x00)                               // Flags (absolute)
	WriteVarInt(buf, 0)                                // Teleport ID
	WritePacket(conn, PID_CB_PlayerPos, buf.Bytes())

	// Step 4: Start encrypted multiplexed tunnel (using password for encryption)
	startMuxTunnel(conn, leftoverReader, password, motion)
}

// startMuxTunnel creates an encrypted yamux session over the Minecraft connection.
// Traffic is encrypted with AES-GCM and disguised as Minecraft chunk data packets.
func startMuxTunnel(conn net.Conn, leftoverReader io.Reader, password string, motion *MotionGenerator) {
	// Use the user's password to derive AES encryption key
	key := sha256.Sum256([]byte(password))
	block, _ := aes.NewCipher(key[:])
	aead, _ := cipher.NewGCM(block)
	pr, pw := io.Pipe()

	mc := &MinecraftConn{conn: conn, r: pr, w: pw, aead: aead, rawReader: leftoverReader, motion: motion}

	go func() {
		defer pw.Close()
		var r io.ByteReader
		if br, ok := leftoverReader.(*bufio.Reader); ok {
			r = br
		} else {
			r = bufio.NewReader(leftoverReader)
		}

		for {
			length, err := ReadVarInt(r)
			if err != nil {
				return
			}
			data := make([]byte, length)
			_, err = io.ReadFull(leftoverReader, data)
			if err != nil {
				return
			}
			pBuf := bytes.NewBuffer(data)
			pid, _ := ReadVarInt(pBuf)

			if pid == PID_SB_PluginMsg {
				channel, _ := ReadString(pBuf)
				if channel == "minecraft:brand" || channel == "minewire:tunnel" {
					enc := pBuf.Bytes()
					if len(enc) < aead.NonceSize() {
						continue
					}
					nonce := enc[:aead.NonceSize()]
					pt, err := aead.Open(nil, nonce, enc[aead.NonceSize():], nil)
					if err == nil {
						pw.Write(pt)
					}
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		timeTicker := time.NewTicker(20 * time.Second) // Minecraft time flows...
		defer ticker.Stop()
		defer timeTicker.Stop()

		worldTime := int64(0)

		for {
			select {
			case <-ticker.C:
				buf := new(bytes.Buffer)
				WriteLong(buf, time.Now().UnixNano())
				WritePacket(conn, PID_CB_KeepAlive, buf.Bytes())
			case <-timeTicker.C:
				// Send Time Update to encourage client simulation
				worldTime += 20 * 20 // Advance 20 seconds (20 ticks/sec)
				buf := new(bytes.Buffer)
				WriteLong(buf, worldTime)        // World Age
				WriteLong(buf, -worldTime%24000) // Time of day (negative to stop internal cycle if client respected it, but here just updating)
				WritePacket(conn, PID_CB_TimeUpdate, buf.Bytes())

				// Update motion simulation rarely to be efficient
				mc.motion.Update()
			}
		}
	}()

	session, err := yamux.Server(mc, nil)
	if err != nil {
		return
	}

	for {
		stream, err := session.Accept()
		if err != nil {
			return
		}
		go handleStream(stream)
	}
}

// handleStream handles a single multiplexed stream by proxying it to the requested destination.
func handleStream(stream net.Conn) {
	defer stream.Close()
	br := bufio.NewReader(stream)
	dest, err := ReadString(br)
	if err != nil {
		return
	}

	target, err := net.DialTimeout("tcp", dest, 10*time.Second)
	if err != nil {
		return
	}
	defer target.Close()

	// Bidirectional copy between stream and target
	done := make(chan bool, 2)
	go func() { io.Copy(target, br); done <- true }()
	go func() { io.Copy(stream, target); done <- true }()
	<-done
}

// MinecraftConn wraps a net.Conn to encrypt/decrypt data and disguise it as Minecraft packets.

type MinecraftConn struct {
	conn      net.Conn
	r         *io.PipeReader
	w         *io.PipeWriter
	aead      cipher.AEAD
	rawReader io.Reader
	motion    *MotionGenerator
}

func (mc *MinecraftConn) Read(b []byte) (int, error) { return mc.r.Read(b) }

// Write encrypts data and wraps it in a realistic Minecraft chunk data packet.
func (mc *MinecraftConn) Write(b []byte) (int, error) {
	nonce := make([]byte, mc.aead.NonceSize())
	rand.Read(nonce)
	encrypted := mc.aead.Seal(nonce, nonce, b, nil)

	buf := new(bytes.Buffer)

	// Use simulated coordinates for Chunk X/Z based on current player position
	// This makes the "chunks" appear around the player
	chunkX := int(mc.motion.X) >> 4
	chunkZ := int(mc.motion.Z) >> 4

	WriteInt(buf, int32(chunkX)) // Chunk X
	WriteInt(buf, int32(chunkZ)) // Chunk Z

	// Add realistic NBT heightmap data to disguise the packet
	// TAG_Compound (Start)
	buf.WriteByte(0x0A)
	buf.Write([]byte{0x00, 0x00}) // Empty name

	// TAG_Long_Array "MOTION_BLOCKING"
	buf.WriteByte(0x0C) // Type: Long Array
	WriteStringNBT(buf, "MOTION_BLOCKING")
	WriteInt(buf, 37) // Array length: 37 longs

	// Write 37 longs containing packed height data
	// Using constant height of 64 for simplicity
	heights := createPackedHeights(64)
	for _, h := range heights {
		WriteLong(buf, h)
	}

	// TAG_End
	buf.WriteByte(0x00)

	// Add encrypted payload
	WriteVarInt(buf, len(encrypted))
	buf.Write(encrypted)

	// Add empty post-data fields (block entities, light masks)
	WriteVarInt(buf, 0) // Block entities count
	// Light masks (all empty)
	WriteVarInt(buf, 0)
	WriteVarInt(buf, 0)
	WriteVarInt(buf, 0)
	WriteVarInt(buf, 0)
	WriteVarInt(buf, 0)
	WriteVarInt(buf, 0)

	err := WritePacket(mc.conn, PID_CB_ChunkData, buf.Bytes())
	return len(b), err
}

// createPackedHeights generates packed height data for Minecraft chunk heightmaps.
// Each height value is 9 bits, packed into an array of 37 longs.
func createPackedHeights(y int64) [37]int64 {
	var data [37]int64
	for i := 0; i < 256; i++ {
		longIndex := i / 7
		bitOffset := (i % 7) * 9
		value := y & 0x1FF // Mask to 9 bits
		data[longIndex] |= (value << bitOffset)
	}
	return data
}

func WriteStringNBT(w io.Writer, s string) {
	b := []byte(s)
	binary.Write(w, binary.BigEndian, int16(len(b))) // Short Len
	w.Write(b)
}

func (mc *MinecraftConn) Close() error                       { return mc.conn.Close() }
func (mc *MinecraftConn) LocalAddr() net.Addr                { return mc.conn.LocalAddr() }
func (mc *MinecraftConn) RemoteAddr() net.Addr               { return mc.conn.RemoteAddr() }
func (mc *MinecraftConn) SetDeadline(t time.Time) error      { return mc.conn.SetDeadline(t) }
func (mc *MinecraftConn) SetReadDeadline(t time.Time) error  { return mc.conn.SetReadDeadline(t) }
func (mc *MinecraftConn) SetWriteDeadline(t time.Time) error { return mc.conn.SetWriteDeadline(t) }

func sendFakeStatus(conn io.Writer) {
	iconData, _ := os.ReadFile(cfg.IconPath)
	icon64 := ""
	if len(iconData) > 0 {
		icon64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(iconData)
	}

	onlineLock.Lock()
	on := currentOnline
	onlineLock.Unlock()

	resp := StatusResponse{
		Version:     Version{Name: cfg.VersionName, Protocol: cfg.ProtocolID},
		Players:     Players{Max: cfg.MaxPlayers, Online: on},
		Description: Description{Text: cfg.Motd},
		Favicon:     icon64,
	}
	d, _ := json.Marshal(resp)
	b := new(bytes.Buffer)
	WriteString(b, string(d))
	WritePacket(conn, PID_CB_StatusResp, b.Bytes())
}

func sendDisconnect(conn io.Writer, r string) {
	s := fmt.Sprintf(`{"text": "%s"}`, r)
	b := new(bytes.Buffer)
	WriteString(b, s)
	WritePacket(conn, PID_CB_LoginDisconnect, b.Bytes())
}

type StatusResponse struct {
	Version     Version     `json:"version"`
	Players     Players     `json:"players"`
	Description Description `json:"description"`
	Favicon     string      `json:"favicon,omitempty"`
}
type Version struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}
type Players struct {
	Max    int           `json:"max"`
	Online int           `json:"online"`
	Sample []interface{} `json:"sample,omitempty"`
}
type Description struct {
	Text string `json:"text"`
}
