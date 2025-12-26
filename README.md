# GenDrive
> Distributed Cloud Storage System

![License](https://img.shields.io/badge/license-MIT-000000.svg?style=for-the-badge)
![Go](https://img.shields.io/badge/built_with-GO-00ADD8.svg?style=for-the-badge&logo=go&logoColor=white)

GenDrive is a distributed file system implementation that aggregates storage capacity from multiple disparate devices, personal computers, servers, and edge nodes, into a single, unified cloud storage layer. It eliminates the need for centralized data silos by leveraging underutilized local storage across a private network mesh.

The system is designed with a focus on resilience, data sovereignty, and minimal configuration, acting as a private "cloud" infrastructure on your own hardware.

---

### System Architecture

The technical architecture constitutes two primary planes:

*   **Control Plane (Orchestrator)**: A centralized Go-based server responsible for metadata management, user authentication, and cluster orchestration. It maintains a ledger of file-to-chunk mappings and device states (SQLite) but does not persistently store user data.
*   **Data Plane (Storage Mesh)**: A network of lightweight agents running on distributed nodes. These agents handle standard I/O operations, managing the storage and retrieval of encrypted data chunks.
*   **Relay Protocol**: A real-time command-and-control channel that facilitates NAT traversal and orchestrates peer-to-peer data transfer commands (Store, Retrieve, Delete).

### Core Capabilities

*   **Unified Cloud Filesystem**: Abstraction of physical storage devices into a single virtual drive locatable via the web dashboard.
*   **Automated Distribution**: Files are ingested, sharded, and mathematically distributed across available nodes based on capacity and load metrics.
*   **Dynamic Rebalancing**: The orchestrator continuously monitors node health and storage utilization, automatically migrating chunks from over-utilized to under-utilized nodes to ensure optimal cluster performance.
*   **Industrial Dashboard**: A minimal, performance-first web interface for fleet management, file operations, and real-time system introspection.

### Quick Start Guide

#### 1. Deploy the Orchestrator
The control server manages the swarm. It requires Go installed on the host machine.

```bash
cd server
go run main.go
# Server listens on port 8080 by default.
```

#### 2. Initialize a Storage Node
Deploy the agent on any Windows machine to join it to the storage mesh.

**PowerShell (Admin recommended):**
```powershell
irm http://localhost:8080/install.ps1 | iex
```

#### 3. Cluster Configuration
1.  Access the dashboard at `http://localhost:8080`.
2.  Navigate to the **Network** tab.
3.  Enter the `Device ID` and `Claim Token` displayed on the agent's console to cryptographically link the node to your cluster.
4.  Once verified, the node's storage capacity is immediately available to the pool.

### 🌍 Public Instance (Community Mesh)
You don't need to host the server yourself! You can join the public community mesh.
1. Access the dashboard: **[drive.chirag404.me](http://drive.chirag404.me)**.
2. Sign up / Login.
3. Run the installer on your nodes (it's pre-configured for this domain).

---

### 🖥️ Self-Hosting (Ubuntu Server)
If you prefer total control, you can host the Control Plane on your own VPS (e.g., DigitalOcean, AWS, Hetzner).

**1. Build & Run**
```bash
# Clone the repo
git clone https://github.com/chirag404/gendrive.git
cd gendrive/server

# Build
go build -o server

# Run (Recommendation: Use Systemd or Docker)
./server
```

**2. Configuring the Agent**
If self-hosting, your agents need to know where your server is.
*   **Option A**: Edit `install.ps1` to set `$BaseUrl = "http://YOUR_SERVER_IP:8080"`.
*   **Option B**: Run the agent manually:
    ```powershell
    ./agent.exe -server "http://YOUR_SERVER_IP:8080"
    ```

### API Reference

*   `POST /api/upload`: Ingest a file, shard it, and distribute chunks to the mesh.
*   `GET /api/download?id={file_id}`: Reassemble chunks from the mesh into the original file.
*   `DELETE /api/delete?id={file_id}`: Remove file metadata and trigger garbage collection on storage nodes.
*   `GET /api/devices`: Retrieve specific telemetry and status for all connected nodes.

---

<img width="1204" height="605" alt="image" src="https://github.com/user-attachments/assets/e08c5990-b7d0-4170-a1cc-0aaa5b5961ad" />

<img width="1187" height="902" alt="image" src="https://github.com/user-attachments/assets/05cb3bc8-bebc-4972-923e-6cecbd8f3b41" />
