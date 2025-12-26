async function loadDevices() {
    try {
        const res = await fetch('/peers');
        const devices = await res.json();
        const tbody = document.querySelector('#deviceTable tbody');
        tbody.innerHTML = '';
        devices.forEach(d => {
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>${d.name}</td>
                <td>${d.id}</td>
                <td class="${d.online ? 'online' : 'offline'}">${d.online ? 'Online' : 'Offline'}</td>
                <td>${d.last_seen}</td>
                <td>${d.ip}</td>
            `;
            tbody.appendChild(tr);
        });
    } catch (e) {
        console.error("Failed to load devices", e);
    }
}

async function loadFiles() {
    try {
        const res = await fetch('/metadata');
        const files = await res.json();
        const tbody = document.querySelector('#fileTable tbody');
        tbody.innerHTML = '';
        if (files) {
            files.forEach(f => {
                const tr = document.createElement('tr');
                tr.innerHTML = `
                    <td>${f.path}</td>
                    <td>${f.size}</td>
                    <td>${f.hash.substring(0, 10)}...</td>
                    <td>${f.chunks ? f.chunks.length : '?'}</td>
                `;
                tbody.appendChild(tr);
            });
        }
    } catch (e) {
        console.error("Failed to load files", e);
    }
}

loadDevices();
loadFiles();
setInterval(loadDevices, 5000);
setInterval(loadFiles, 10000);
