# GenDrive

![License](https://img.shields.io/badge/license-MIT-000000.svg?style=for-the-badge)
![Go](https://img.shields.io/badge/built_with-GO-00ADD8.svg?style=for-the-badge&logo=go&logoColor=white)

GenDrive is a distributed file system implementation that aggregates storage capacity from multiple disparate devices, personal computers, servers, and edge nodes, into a single, unified cloud storage layer. It eliminates the need for centralized data silos by leveraging underutilized local storage across a private network mesh.

The system is designed with a focus on resilience, data sovereignty, and minimal configuration, acting as a private "cloud" infrastructure on your own hardware.

ViewDemo: https://drive.chirag404.me
About GenDrive: https://www.chirag404.me/gendrive

<img width="1500" height="900" alt="image" src="https://github.com/user-attachments/assets/91f2c590-5868-4d82-b792-2750d385f38b" />


---
### System Architecture

The system operates on a split-plane architecture designed for privacy and resilience.

*   **Control Plane (Orchestrator)**
    A lightweight central server that manages authentication, metadata, and cluster health. It maintains a ledger of file locations but never stores actual file data or encryption keys.

*   **Data Plane (Storage Mesh)**
    Your interconnected devices act as distributed storage nodes. Files are encrypted client-side, split into shards, and distributed across the mesh.

*   **Relay Protocol**
    A custom protocol tailored for high-throughput peer-to-peer data transfer, capable of traversing complex network topologies (NATs) without manual port forwarding.

### Core Capabilities

*   **Unified Filesystem**: Aggregates storage capacity from disparate devices into a single, addressable virtual drive.
*   **Zero-Knowledge Encryption**: Data is encrypted using AES-256 before leaving the source device. The orchestrator handles only opaque binary blobs.
*   **Dynamic Rebalancing**: The system continuously monitors node health and storage utilization, automatically migrating data to ensure optimal distribution and redundancy.
*   **Industrial Interface**: A low-latency, strictly functional web dashboard for fleet management and file operations.

### Supported Agents

The agent binary is designed to be portable and dependency-free.

*   **Windows**: Recommended for primary storage nodes. Installing via the provided PowerShell script handles persistence automatically.
*   **Linux / macOS**: Supported for headless operation on servers, VPS instances, or Raspberry Pis. Requires manual binary execution.

---

### Community Registry

You can join the public community mesh to start using the system immediately without infrastructure setup.

1.  **Register**: Create an account at [drive.chirag404.me](http://drive.chirag404.me).
2.  **Deploy**: Run the installer command on your devices to link them to your account.
3.  **Use**: Your local storage is now part of your private cloud.

---

### Self-Hosting Guide

For complete control over the metadata and orchestration layer, you can host the server on any standard Linux VPS.

**1. Deployment**

Clone the repository and build the server binary.

```bash
git clone https://github.com/chirag404/gendrive.git
cd gendrive/server
go build -o server
./server
```

**2. Agent Configuration**

To connect agents to your self-hosted instance, direct the agent to your server's IP address.

*   **PowerShell**: Update the `$BaseUrl` variable in `install.ps1`.
*   **Manual**: Run the agent with the server flag:
    `./agent.exe -server "http://YOUR_IP:8080"`

### API Reference

*   `POST /api/upload`: Accepts a file stream, performs sharding, and distributes chunks to active nodes.
*   `GET /api/download`: Retrieval endpoint that reassembles distributed chunks into the original file.
*   `DELETE /api/delete`: Removes file metadata and issues garbage collection commands to storage nodes.
*   `GET /api/devices`: returns telemetry data including storage usage, connection status, and IP info.

---

Test the app at https://drive.chirag404.me !!
