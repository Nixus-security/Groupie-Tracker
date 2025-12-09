/**
 * Gestion des salles
 */
class RoomManager {
  constructor(roomCode) {
    this.roomCode = roomCode;
    this.ws = null;
    this.isHost = false;
    this.isReady = false;
    this.players = [];
    
    this.init();
  }

  /**
   * Initialise la salle
   */
  init() {
    // Éléments DOM
    this.elements = {
      playersList: document.getElementById('players-list'),
      readyBtn: document.getElementById('ready-btn'),
      startBtn: document.getElementById('start-btn'),
      leaveBtn: document.getElementById('leave-btn'),
      configForm: document.getElementById('config-form'),
      roomCode: document.getElementById('room-code'),
      roomStatus: document.getElementById('room-status')
    };

    // Afficher le code
    if (this.elements.roomCode) {
      this.elements.roomCode.textContent = this.roomCode;
    }

    // Connexion WebSocket
    this.ws = new GameWebSocket(this.roomCode, {
      onConnect: () => this.onConnect(),
      onDisconnect: () => this.onDisconnect(),
      onError: (err) => this.onError(err),
      
      onRoomUpdate: (data) => this.onRoomUpdate(data),
      onPlayerJoined: (data) => this.onPlayerJoined(data),
      onPlayerLeft: (data) => this.onPlayerLeft(data),
      onPlayerReady: (data) => this.onPlayerReady(data),
      onStartGame: (data) => this.onStartGame(data)
    });

    this.ws.connect();

    // Événements boutons
    if (this.elements.readyBtn) {
      this.elements.readyBtn.addEventListener('click', () => this.toggleReady());
    }
    
    if (this.elements.startBtn) {
      this.elements.startBtn.addEventListener('click', () => this.startGame());
    }
    
    if (this.elements.leaveBtn) {
      this.elements.leaveBtn.addEventListener('click', () => this.leaveRoom());
    }

    // Copier le code au clic
    if (this.elements.roomCode) {
      this.elements.roomCode.addEventListener('click', () => {
        navigator.clipboard.writeText(this.roomCode).then(() => {
          this.showMessage('Code copié !', 'success');
        });
      });
      this.elements.roomCode.style.cursor = 'pointer';
      this.elements.roomCode.title = 'Cliquez pour copier';
    }
  }

  // === Événements WebSocket ===

  onConnect() {
    console.log('✅ Connecté à la salle');
    this.showMessage('Connecté à la salle', 'success');
  }

  onDisconnect() {
    console.log('❌ Déconnecté');
    this.showMessage('Déconnecté du serveur...', 'danger');
  }

  onError(err) {
    console.error('Erreur:', err);
    this.showMessage(err.message || 'Erreur', 'danger');
  }

  /**
   * Mise à jour complète de la salle
   */
  onRoomUpdate(data) {
    console.log('🏠 Mise à jour salle:', data);
    
    this.players = data.players || [];
    this.isHost = data.host_id === this.getCurrentUserId();
    
    // Trouver si on est prêt
    const currentPlayer = this.players.find(p => p.user_id === this.getCurrentUserId());
    if (currentPlayer) {
      this.isReady = currentPlayer.is_ready;
    }
    
    this.updateUI(data);
  }

  onPlayerJoined(data) {
    console.log('👋 Joueur rejoint:', data.pseudo);
    this.showMessage(`${data.pseudo} a rejoint la salle`, 'info');
  }

  onPlayerLeft(data) {
    console.log('👋 Joueur parti:', data.pseudo);
    this.showMessage(`${data.pseudo} a quitté la salle`, 'warning');
  }

  onPlayerReady(data) {
    console.log('✓ Joueur prêt:', data.pseudo, data.ready);
    
    // Mettre à jour le joueur
    const player = this.players.find(p => p.user_id === data.user_id);
    if (player) {
      player.is_ready = data.ready;
      this.updatePlayersList();
    }
    
    this.showMessage(
      `${data.pseudo} ${data.ready ? 'est prêt' : "n'est plus prêt"}`,
      data.ready ? 'success' : 'info'
    );
  }

  onStartGame(data) {
    console.log('🎮 Partie lancée:', data);
    this.showMessage('La partie va commencer !', 'success');
    
    // Rediriger vers la page du jeu
    const gameType = data.game_type;
    setTimeout(() => {
      if (gameType === 'blindtest') {
        window.location.href = `/blindtest/${this.roomCode}`;
      } else if (gameType === 'petitbac') {
        window.location.href = `/petitbac/${this.roomCode}`;
      }
    }, 1000);
  }

  // === Actions ===

  toggleReady() {
    this.isReady = !this.isReady;
    this.ws.setReady(this.isReady);
    this.updateReadyButton();
  }

  startGame() {
    if (!this.isHost) {
      this.showMessage("Seul l'hôte peut démarrer la partie", 'warning');
      return;
    }
    
    this.ws.startGame();
  }

  leaveRoom() {
    if (confirm('Voulez-vous vraiment quitter la salle ?')) {
      this.ws.leaveRoom();
      window.location.href = '/rooms';
    }
  }

  // === UI ===

  updateUI(data) {
    this.updatePlayersList();
    this.updateButtons(data.is_ready);
    this.updateStatus(data.status);
  }

  updatePlayersList() {
    if (!this.elements.playersList) return;
    
    let html = '';
    
    this.players.forEach(p => {
      const isCurrentUser = p.user_id === this.getCurrentUserId();
      html += `
        <div class="player-item ${p.is_host ? 'host' : ''} ${p.is_ready ? 'ready' : ''}">
          <div class="player-info">
            <div class="player-avatar">${p.pseudo.charAt(0).toUpperCase()}</div>
            <div>
              <div>${p.pseudo} ${isCurrentUser ? '(vous)' : ''}</div>
              <div class="player-status">
                ${p.is_host ? '👑 Hôte' : ''}
                ${p.is_ready ? '✅ Prêt' : '⏳ En attente'}
              </div>
            </div>
          </div>
          <div class="connection-status">
            ${p.connected ? '🟢' : '🔴'}
          </div>
        </div>
      `;
    });
    
    this.elements.playersList.innerHTML = html;
  }

  updateReadyButton() {
    if (!this.elements.readyBtn) return;
    
    if (this.isReady) {
      this.elements.readyBtn.textContent = 'Annuler';
      this.elements.readyBtn.classList.remove('btn-success');
      this.elements.readyBtn.classList.add('btn-secondary');
    } else {
      this.elements.readyBtn.textContent = 'Prêt !';
      this.elements.readyBtn.classList.remove('btn-secondary');
      this.elements.readyBtn.classList.add('btn-success');
    }
  }

  updateButtons(isRoomReady) {
    this.updateReadyButton();
    
    if (this.elements.startBtn) {
      this.elements.startBtn.style.display = this.isHost ? 'inline-block' : 'none';
      this.elements.startBtn.disabled = !isRoomReady;
    }
  }

  updateStatus(status) {
    if (!this.elements.roomStatus) return;
    
    const statusTexts = {
      'waiting': '⏳ En attente des joueurs',
      'playing': '🎮 Partie en cours',
      'finished': '🏁 Partie terminée'
    };
    
    this.elements.roomStatus.textContent = statusTexts[status] || status;
  }

  getCurrentUserId() {
    if (window.currentUserId) return window.currentUserId;
    const userEl = document.querySelector('[data-current-user-id]');
    if (userEl) return parseInt(userEl.dataset.currentUserId);
    return null;
  }

  showMessage(message, type = 'info') {
    const notif = document.createElement('div');
    notif.className = `alert alert-${type} fade-in`;
    notif.textContent = message;
    notif.style.position = 'fixed';
    notif.style.top = '20px';
    notif.style.right = '20px';
    notif.style.zIndex = '1000';
    
    document.body.appendChild(notif);
    
    setTimeout(() => {
      notif.remove();
    }, 3000);
  }
}

// Initialisation automatique
document.addEventListener('DOMContentLoaded', () => {
  const roomCodeEl = document.getElementById('room-code');
  const roomCode = roomCodeEl ? roomCodeEl.textContent.trim() : 
                   new URLSearchParams(window.location.search).get('room');
  
  if (roomCode && document.getElementById('players-list')) {
    window.roomManager = new RoomManager(roomCode);
  }
});