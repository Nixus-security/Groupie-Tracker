class WebSocketManager {
    constructor() {
        this.ws = null;
        this.roomCode = null;
        this.userId = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 1000;
        this.handlers = new Map();
        this.isConnected = false;
        this.pingInterval = null;
    }

    connect(roomCode, userId) {
        this.roomCode = roomCode;
        this.userId = userId;

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/room/${roomCode}`;

        console.log('🔌 Connexion WebSocket:', wsUrl);

        try {
            this.ws = new WebSocket(wsUrl);
            this.setupEventListeners();
        } catch (error) {
            console.error('❌ Erreur création WebSocket:', error);
            this.handleReconnect();
        }
    }

    setupEventListeners() {
        this.ws.onopen = () => {
            console.log('✅ WebSocket connecté');
            this.isConnected = true;
            this.reconnectAttempts = 0;
            this.startPingInterval();
            this.emit('connected');
            showToast('Connecté à la salle', 'success');
        };

        this.ws.onclose = (event) => {
            console.log('🔌 WebSocket fermé:', event.code, event.reason);
            this.isConnected = false;
            this.stopPingInterval();
            this.emit('disconnected');
            
            if (!event.wasClean) {
                this.handleReconnect();
            }
        };

        this.ws.onerror = (error) => {
            console.error('❌ Erreur WebSocket:', error);
            this.emit('error', error);
        };

        this.ws.onmessage = (event) => {
            try {
                const messages = event.data.split('\n');
                messages.forEach(msgStr => {
                    if (msgStr.trim()) {
                        const message = JSON.parse(msgStr);
                        this.handleMessage(message);
                    }
                });
            } catch (error) {
                console.error('❌ Erreur parsing message:', error);
            }
        };
    }

    handleMessage(message) {
        console.log('📨 Message reçu:', message.type, message);

        this.emit(message.type, message.payload);

        if (message.type === 'error') {
            console.error('❌ Erreur serveur:', message.error);
            showToast(message.error, 'error');
        }

        if (message.type === 'pong') {
            console.log('🏓 Pong reçu');
        }
    }

    send(type, payload = {}) {
        if (!this.isConnected || !this.ws) {
            console.warn('⚠️ WebSocket non connecté');
            return false;
        }

        const message = { type, payload };
        
        try {
            this.ws.send(JSON.stringify(message));
            console.log('📤 Message envoyé:', type, payload);
            return true;
        } catch (error) {
            console.error('❌ Erreur envoi message:', error);
            return false;
        }
    }

    on(type, handler) {
        if (!this.handlers.has(type)) {
            this.handlers.set(type, []);
        }
        this.handlers.get(type).push(handler);
    }

    off(type, handler) {
        if (this.handlers.has(type)) {
            const handlers = this.handlers.get(type);
            const index = handlers.indexOf(handler);
            if (index > -1) {
                handlers.splice(index, 1);
            }
        }
    }

    emit(type, data) {
        if (this.handlers.has(type)) {
            this.handlers.get(type).forEach(handler => {
                try {
                    handler(data);
                } catch (error) {
                    console.error(`❌ Erreur handler ${type}:`, error);
                }
            });
        }
    }

    handleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('❌ Nombre max de tentatives de reconnexion atteint');
            showToast('Connexion perdue. Veuillez rafraîchir la page.', 'error');
            return;
        }

        this.reconnectAttempts++;
        const delay = this.reconnectDelay * this.reconnectAttempts;

        console.log(`🔄 Tentative de reconnexion ${this.reconnectAttempts}/${this.maxReconnectAttempts} dans ${delay}ms`);
        showToast(`Reconnexion en cours... (${this.reconnectAttempts}/${this.maxReconnectAttempts})`, 'warning');

        setTimeout(() => {
            this.connect(this.roomCode, this.userId);
        }, delay);
    }

    startPingInterval() {
        this.pingInterval = setInterval(() => {
            if (this.isConnected) {
                this.send('ping');
            }
        }, 30000); // Ping toutes les 30 secondes
    }

    stopPingInterval() {
        if (this.pingInterval) {
            clearInterval(this.pingInterval);
            this.pingInterval = null;
        }
    }

    disconnect() {
        this.stopPingInterval();
        if (this.ws) {
            this.ws.close(1000, 'Déconnexion volontaire');
            this.ws = null;
        }
        this.isConnected = false;
    }


    setReady(ready) {
        this.send('player_ready', { ready });
    }

    leaveRoom() {
        this.send('leave_room');
    }

    startGame() {
        this.send('start_game');
    }

    submitBlindTestAnswer(answer) {
        this.send('bt_answer', { answer });
    }

    submitPetitBacAnswers(answers) {
        this.send('pb_answer', { answers });
    }

    submitPetitBacVote(targetUserId, category, isValid) {
        this.send('pb_vote', {
            target_user_id: targetUserId,
            category: category,
            is_valid: isValid
        });
    }

    stopPetitBacRound() {
        this.send('pb_stop_round');
    }
}

const wsManager = new WebSocketManager();


function initRoom(roomCode, userId) {
    wsManager.connect(roomCode, userId);
    setupRoomHandlers();
}

function setupRoomHandlers() {
    wsManager.on('room_update', (data) => {
        updateRoomUI(data);
    });

    wsManager.on('player_joined', (data) => {
        showToast(`${data.pseudo} a rejoint la salle`, 'info');
        addPlayerToList(data);
    });

    wsManager.on('player_left', (data) => {
        showToast(`${data.pseudo} a quitté la salle`, 'warning');
        removePlayerFromList(data.user_id);
    });

    wsManager.on('player_ready', (data) => {
        updatePlayerReady(data.user_id, data.ready);
        if (data.ready) {
            showToast(`${data.pseudo} est prêt !`, 'success');
        }
    });

    wsManager.on('start_game', (data) => {
        showToast('La partie commence !', 'success');
        handleGameStart(data);
    });
}

function updateRoomUI(data) {
    const playersList = document.getElementById('players-list');
    if (playersList && data.players) {
        playersList.innerHTML = '';
        data.players.forEach(player => {
            addPlayerToList(player);
        });
    }

    const startBtn = document.getElementById('start-btn');
    if (startBtn) {
        startBtn.disabled = !data.is_ready;
    }

    const statusBadge = document.querySelector('.room-status');
    if (statusBadge) {
        statusBadge.textContent = getStatusText(data.status);
        statusBadge.className = `badge status-${data.status}`;
    }
}

function addPlayerToList(player) {
    const playersList = document.getElementById('players-list');
    if (!playersList) return;

    let playerCard = document.querySelector(`[data-user-id="${player.user_id}"]`);
    
    if (!playerCard) {
        playerCard = document.createElement('div');
        playerCard.className = 'player-card';
        playerCard.dataset.userId = player.user_id;
        playersList.appendChild(playerCard);
    }

    playerCard.innerHTML = `
        <div class="player-avatar">${player.pseudo.charAt(0).toUpperCase()}</div>
        <div class="player-info">
            <div class="player-name">
                ${player.pseudo}
                ${player.is_host ? '<span class="badge badge-warning">👑 Hôte</span>' : ''}
            </div>
            <div class="player-status">
                ${player.is_ready ? '✅ Prêt' : '⏳ En attente'}
            </div>
        </div>
        ${player.score !== undefined ? `<div class="player-score">${player.score} pts</div>` : ''}
    `;

    playerCard.classList.toggle('host', player.is_host);
    playerCard.classList.toggle('ready', player.is_ready);
    playerCard.classList.toggle('disconnected', !player.connected);
}

function removePlayerFromList(userId) {
    const playerCard = document.querySelector(`[data-user-id="${userId}"]`);
    if (playerCard) {
        playerCard.classList.add('animate-fade-out');
        setTimeout(() => playerCard.remove(), 300);
    }
}

function updatePlayerReady(userId, ready) {
    const playerCard = document.querySelector(`[data-user-id="${userId}"]`);
    if (playerCard) {
        playerCard.classList.toggle('ready', ready);
        const statusEl = playerCard.querySelector('.player-status');
        if (statusEl) {
            statusEl.textContent = ready ? '✅ Prêt' : '⏳ En attente';
        }
    }
}

function getStatusText(status) {
    const statusTexts = {
        'waiting': 'En attente',
        'playing': 'En cours',
        'finished': 'Terminée'
    };
    return statusTexts[status] || status;
}

function handleGameStart(data) {
    const gameType = data.game_type;
    
    if (gameType === 'blindtest') {
        if (typeof initBlindTest === 'function') {
            initBlindTest(data.config);
        } else {
            window.location.href = `/blindtest/${wsManager.roomCode}`;
        }
    } else if (gameType === 'petitbac') {
        if (typeof initPetitBac === 'function') {
            initPetitBac(data.config);
        } else {
            window.location.href = `/petitbac/${wsManager.roomCode}`;
        }
    }
}


function showToast(message, type = 'info', duration = 3000) {
    let container = document.querySelector('.toast-container');
    if (!container) {
        container = document.createElement('div');
        container.className = 'toast-container';
        document.body.appendChild(container);
    }

    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    
    const icons = {
        success: '✓',
        error: '✕',
        warning: '⚠',
        info: 'ℹ'
    };

    toast.innerHTML = `
        <span class="toast-icon">${icons[type] || icons.info}</span>
        <span class="toast-message">${message}</span>
    `;

    container.appendChild(toast);

    setTimeout(() => {
        toast.classList.add('toast-exit');
        setTimeout(() => toast.remove(), 300);
    }, duration);
}


function copyRoomCode(code) {
    navigator.clipboard.writeText(code).then(() => {
        showToast('Code copié !', 'success');
    }).catch(() => {
        showToast('Erreur lors de la copie', 'error');
    });
}

function formatTime(seconds) {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
}

function animateElement(element, animationClass) {
    element.classList.add(animationClass);
    element.addEventListener('animationend', () => {
        element.classList.remove(animationClass);
    }, { once: true });
}

function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

window.wsManager = wsManager;
window.initRoom = initRoom;
window.showToast = showToast;
window.copyRoomCode = copyRoomCode;
window.formatTime = formatTime;