/**
 * ============================================================================
 * GROUPIE-TRACKER - WebSocket Manager
 * ============================================================================
 */

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

    /**
     * Connecte au WebSocket pour une salle
     * @param {string} roomCode - Code de la salle
     * @param {number} userId - ID de l'utilisateur
     */
    connect(roomCode, userId) {
        this.roomCode = roomCode;
        this.userId = userId;

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws?room=${roomCode}`;

        console.log('🔌 Connexion WebSocket:', wsUrl);

        try {
            this.ws = new WebSocket(wsUrl);
            this.setupEventListeners();
        } catch (error) {
            console.error('❌ Erreur création WebSocket:', error);
            this.handleReconnect();
        }
    }

    /**
     * Configure les listeners d'événements WebSocket
     */
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

    /**
     * Gère les messages reçus
     * @param {Object} message - Message WebSocket
     */
    handleMessage(message) {
        console.log('📨 Message reçu:', message.type, message);

        // Émettre l'événement correspondant au type de message
        this.emit(message.type, message.payload);

        // Gérer les messages d'erreur
        if (message.type === 'error') {
            console.error('❌ Erreur serveur:', message.error);
            showToast(message.error, 'error');
        }

        // Répondre aux pongs
        if (message.type === 'pong') {
            console.log('🏓 Pong reçu');
        }
    }

    /**
     * Envoie un message WebSocket
     * @param {string} type - Type du message
     * @param {Object} payload - Données du message
     */
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

    /**
     * Enregistre un handler pour un type de message
     * @param {string} type - Type du message
     * @param {Function} handler - Fonction de callback
     */
    on(type, handler) {
        if (!this.handlers.has(type)) {
            this.handlers.set(type, []);
        }
        this.handlers.get(type).push(handler);
    }

    /**
     * Supprime un handler
     * @param {string} type - Type du message
     * @param {Function} handler - Fonction à supprimer
     */
    off(type, handler) {
        if (this.handlers.has(type)) {
            const handlers = this.handlers.get(type);
            const index = handlers.indexOf(handler);
            if (index > -1) {
                handlers.splice(index, 1);
            }
        }
    }

    /**
     * Émet un événement aux handlers enregistrés
     * @param {string} type - Type d'événement
     * @param {*} data - Données de l'événement
     */
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

    /**
     * Gère la reconnexion automatique
     */
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

    /**
     * Démarre l'intervalle de ping
     */
    startPingInterval() {
        this.pingInterval = setInterval(() => {
            if (this.isConnected) {
                this.send('ping');
            }
        }, 30000); // Ping toutes les 30 secondes
    }

    /**
     * Arrête l'intervalle de ping
     */
    stopPingInterval() {
        if (this.pingInterval) {
            clearInterval(this.pingInterval);
            this.pingInterval = null;
        }
    }

    /**
     * Ferme la connexion WebSocket
     */
    disconnect() {
        this.stopPingInterval();
        if (this.ws) {
            this.ws.close(1000, 'Déconnexion volontaire');
            this.ws = null;
        }
        this.isConnected = false;
    }

    // =========================================================================
    // MÉTHODES SPÉCIFIQUES AU JEU
    // =========================================================================

    /**
     * Signale que le joueur est prêt
     * @param {boolean} ready - État prêt
     */
    setReady(ready) {
        this.send('player_ready', { ready });
    }

    /**
     * Quitte la salle
     */
    leaveRoom() {
        this.send('leave_room');
    }

    /**
     * Démarre la partie (hôte uniquement)
     */
    startGame() {
        this.send('start_game');
    }

    /**
     * Envoie une réponse Blind Test
     * @param {string} answer - Réponse du joueur
     */
    submitBlindTestAnswer(answer) {
        this.send('bt_answer', { answer });
    }

    /**
     * Envoie les réponses Petit Bac
     * @param {Object} answers - Réponses par catégorie
     */
    submitPetitBacAnswers(answers) {
        this.send('pb_answer', { answers });
    }

    /**
     * Envoie un vote Petit Bac
     * @param {number} targetUserId - ID du joueur cible
     * @param {string} category - Catégorie
     * @param {boolean} isValid - Validité de la réponse
     */
    submitPetitBacVote(targetUserId, category, isValid) {
        this.send('pb_vote', {
            target_user_id: targetUserId,
            category: category,
            is_valid: isValid
        });
    }

    /**
     * Stoppe la manche Petit Bac
     */
    stopPetitBacRound() {
        this.send('pb_stop_round');
    }
}

// Instance globale
const wsManager = new WebSocketManager();

// ============================================================================
// FONCTIONS UTILITAIRES
// ============================================================================

/**
 * Initialise la connexion WebSocket pour une salle
 * @param {string} roomCode - Code de la salle
 * @param {number} userId - ID de l'utilisateur
 */
function initRoom(roomCode, userId) {
    wsManager.connect(roomCode, userId);
    setupRoomHandlers();
}

/**
 * Configure les handlers de base pour une salle
 */
function setupRoomHandlers() {
    // Mise à jour de la salle
    wsManager.on('room_update', (data) => {
        updateRoomUI(data);
    });

    // Joueur rejoint
    wsManager.on('player_joined', (data) => {
        showToast(`${data.pseudo} a rejoint la salle`, 'info');
        addPlayerToList(data);
    });

    // Joueur parti
    wsManager.on('player_left', (data) => {
        showToast(`${data.pseudo} a quitté la salle`, 'warning');
        removePlayerFromList(data.user_id);
    });

    // Joueur prêt
    wsManager.on('player_ready', (data) => {
        updatePlayerReady(data.user_id, data.ready);
        if (data.ready) {
            showToast(`${data.pseudo} est prêt !`, 'success');
        }
    });

    // Démarrage du jeu
    wsManager.on('start_game', (data) => {
        showToast('La partie commence !', 'success');
        handleGameStart(data);
    });
}

/**
 * Met à jour l'interface de la salle
 * @param {Object} data - Données de la salle
 */
function updateRoomUI(data) {
    // Mettre à jour la liste des joueurs
    const playersList = document.getElementById('players-list');
    if (playersList && data.players) {
        playersList.innerHTML = '';
        data.players.forEach(player => {
            addPlayerToList(player);
        });
    }

    // Mettre à jour le bouton de démarrage
    const startBtn = document.getElementById('start-btn');
    if (startBtn) {
        startBtn.disabled = !data.is_ready;
    }

    // Mettre à jour le statut
    const statusBadge = document.querySelector('.room-status');
    if (statusBadge) {
        statusBadge.textContent = getStatusText(data.status);
        statusBadge.className = `badge status-${data.status}`;
    }
}

/**
 * Ajoute un joueur à la liste
 * @param {Object} player - Données du joueur
 */
function addPlayerToList(player) {
    const playersList = document.getElementById('players-list');
    if (!playersList) return;

    // Vérifier si le joueur existe déjà
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

/**
 * Supprime un joueur de la liste
 * @param {number} userId - ID du joueur
 */
function removePlayerFromList(userId) {
    const playerCard = document.querySelector(`[data-user-id="${userId}"]`);
    if (playerCard) {
        playerCard.classList.add('animate-fade-out');
        setTimeout(() => playerCard.remove(), 300);
    }
}

/**
 * Met à jour l'état "prêt" d'un joueur
 * @param {number} userId - ID du joueur
 * @param {boolean} ready - État prêt
 */
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

/**
 * Retourne le texte du statut
 * @param {string} status - Code du statut
 * @returns {string} Texte du statut
 */
function getStatusText(status) {
    const statusTexts = {
        'waiting': 'En attente',
        'playing': 'En cours',
        'finished': 'Terminée'
    };
    return statusTexts[status] || status;
}

/**
 * Gère le démarrage d'une partie
 * @param {Object} data - Données du jeu
 */
function handleGameStart(data) {
    const gameType = data.game_type;
    
    if (gameType === 'blindtest') {
        // Rediriger ou initialiser le Blind Test
        if (typeof initBlindTest === 'function') {
            initBlindTest(data.config);
        } else {
            window.location.href = `/blindtest/${wsManager.roomCode}`;
        }
    } else if (gameType === 'petitbac') {
        // Rediriger ou initialiser le Petit Bac
        if (typeof initPetitBac === 'function') {
            initPetitBac(data.config);
        } else {
            window.location.href = `/petitbac/${wsManager.roomCode}`;
        }
    }
}

// ============================================================================
// SYSTÈME DE TOAST NOTIFICATIONS
// ============================================================================

/**
 * Affiche une notification toast
 * @param {string} message - Message à afficher
 * @param {string} type - Type de notification (success, error, warning, info)
 * @param {number} duration - Durée d'affichage en ms
 */
function showToast(message, type = 'info', duration = 3000) {
    // Créer le conteneur si nécessaire
    let container = document.querySelector('.toast-container');
    if (!container) {
        container = document.createElement('div');
        container.className = 'toast-container';
        document.body.appendChild(container);
    }

    // Créer le toast
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

    // Animation de sortie et suppression
    setTimeout(() => {
        toast.classList.add('toast-exit');
        setTimeout(() => toast.remove(), 300);
    }, duration);
}

// ============================================================================
// UTILITAIRES
// ============================================================================

/**
 * Copie le code de la salle dans le presse-papier
 * @param {string} code - Code à copier
 */
function copyRoomCode(code) {
    navigator.clipboard.writeText(code).then(() => {
        showToast('Code copié !', 'success');
    }).catch(() => {
        showToast('Erreur lors de la copie', 'error');
    });
}

/**
 * Formate un temps en secondes en MM:SS
 * @param {number} seconds - Temps en secondes
 * @returns {string} Temps formaté
 */
function formatTime(seconds) {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
}

/**
 * Anime un élément avec une classe
 * @param {HTMLElement} element - Élément à animer
 * @param {string} animationClass - Classe d'animation
 */
function animateElement(element, animationClass) {
    element.classList.add(animationClass);
    element.addEventListener('animationend', () => {
        element.classList.remove(animationClass);
    }, { once: true });
}

/**
 * Debounce une fonction
 * @param {Function} func - Fonction à debouncer
 * @param {number} wait - Délai en ms
 * @returns {Function} Fonction debouncée
 */
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

// Export pour utilisation dans d'autres scripts
window.wsManager = wsManager;
window.initRoom = initRoom;
window.showToast = showToast;
window.copyRoomCode = copyRoomCode;
window.formatTime = formatTime;