# Minewire Server

Proxy server that masquerades as a Minecraft server to establish encrypted tunnels and bypass network restrictions.

## Features

- **AES-GCM Encryption** - All traffic encrypted using client password
- **Minecraft Camouflage** - Appears as legitimate Minecraft server when scanned
- **Stream Multiplexing** - Multiple connections through single tunnel (yamux)
- **Player Simulation** - Realistic online player count fluctuation
- **Password Authentication** - Multi-user support with individual passwords

## How It Works

The protocol leverages Minecraft's packet structure for stealth operation:

1. **Initial Handshake**: Client connects and performs standard Minecraft handshake/status/login sequence
2. **Authentication**: Username is derived from SHA256(password), validated against server's authorized users map
3. **Protocol Simulation**: Server sends Join Game, Player Position, Keep-Alive, and Time Update packets to maintain appearance
4. **Tunnel Establishment**: After authentication, yamux multiplexed session is initiated over the connection
5. **Traffic Encapsulation**: Data is encrypted with AES-GCM and embedded inside Minecraft Chunk Data packets (0x25)
   - Each chunk uses realistic coordinates based on simulated player position
   - Includes authentic NBT heightmap data (MOTION_BLOCKING tag with packed height values)
   - Encrypted payload follows the heightmap structure
6. **Stream Proxying**: Each yamux stream reads target address and proxies TCP connection bidirectionally

The key insight: Minecraft chunk packets can be arbitrarily large and frequent, making them perfect carriers for encrypted tunnel traffic while maintaining protocol compliance.

## Requirements

- Linux server (Ubuntu, Debian, etc.)
- Go 1.19+ (for compilation)
- Root access (for installation)
- Open port (default 25565)

## Installation

### Quick Install

```bash
cd server
sudo bash setup.sh
```

The script automatically:
1. Checks for Go compiler
2. Compiles the server
3. Creates system user `minewire`
4. Installs binary to `/usr/local/bin/minewire-server`
5. Creates config at `/etc/minewire/server.yaml`
6. Installs systemd service
7. Reloads systemd

### Manual Install

```bash
# Compile
cd minewire
go build -o minewire-server

# Create user
sudo useradd --system --no-create-home --shell /bin/false minewire

# Install binary
sudo install -m 755 minewire-server /usr/local/bin/minewire-server

# Setup config
sudo mkdir -p /etc/minewire
sudo cp server.yaml /etc/minewire/server.yaml
sudo cp server-icon.png /etc/minewire/server-icon.png
sudo chown -R minewire:minewire /etc/minewire
sudo chmod 750 /etc/minewire
sudo chmod 640 /etc/minewire/server.yaml

# Install service
sudo cp minewire-server.service /etc/systemd/system/
sudo systemctl daemon-reload
```

## Configuration

### Generate Secure Passwords

```bash
openssl rand -hex 16
```

Example output: `3d7e8a190604e9da51a3543a23421d20`

### Configuration File

Edit `/etc/minewire/server.yaml`:

```yaml
listen_port: "25565"

passwords:
  - "YOUR_PASSWORD_1"
  - "YOUR_PASSWORD_2"

# Minecraft metadata (for camouflage)
version_name: "1.21.10"
protocol_id: 773
icon_path: "server-icon.png"
motd: "§bMinewire Proxy Server\\n§eSecure Tunnel Active"

# Player simulation
max_players: 20
online_min: 4
online_max: 20
```

### Custom Icon (Optional)

Replace with your 64x64 PNG:

```bash
sudo cp your-icon.png /etc/minewire/server-icon.png
sudo chown minewire:minewire /etc/minewire/server-icon.png
```

## Service Management

```bash
# Start
sudo systemctl start minewire-server

# Stop
sudo systemctl stop minewire-server

# Restart
sudo systemctl restart minewire-server

# Status
sudo systemctl status minewire-server

# View logs
sudo journalctl -u minewire-server -n 50

# Follow logs
sudo journalctl -u minewire-server -f

# Enable auto-start
sudo systemctl enable minewire-server
```

## Firewall Setup

### UFW

```bash
sudo ufw allow 25565/tcp
sudo ufw enable
```

### firewalld

```bash
sudo firewall-cmd --permanent --add-port=25565/tcp
sudo firewall-cmd --reload
```

## Uninstall

```bash
sudo systemctl stop minewire-server
sudo systemctl disable minewire-server
sudo rm /usr/local/bin/minewire-server
sudo rm /etc/systemd/system/minewire-server.service
sudo rm -rf /etc/minewire
sudo userdel minewire
sudo systemctl daemon-reload
```

## Architecture

### Components

- `main.go` - Entry point, connection handling
- `handler.go` - Protocol logic, encryption, tunneling
- `protocol.go` - Minecraft protocol primitives (VarInt, String, etc.)
- `motion.go` - Player movement simulation for realistic chunk coordinates
- `server.yaml` - Server configuration
- `minewire-server.service` - systemd service unit
- `setup.sh` - Installation script

### Protocol Details

**Authentication**: Client hashes password with SHA256, takes first 8 hex chars, prefixes with "Player" to generate username. Server validates against pre-computed map.

**Encryption**: Per-user AES-GCM key derived from SHA256(password). Each write generates random nonce, encrypts data, prepends nonce to ciphertext.

**Packet Structure**: Chunk Data (0x25) format:
- Chunk X/Z coordinates (based on simulated player position)
- NBT heightmap compound tag with MOTION_BLOCKING long array (37 longs, 9-bit packed heights)
- VarInt length + encrypted payload
- Empty block entities and light mask arrays

**Motion Simulation**: Random walk algorithm with terrain-following Y-coordinate adjustment. Updates periodically to generate varied chunk coordinates, enhancing camouflage.

## License

MIT