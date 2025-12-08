/**
 * Client WebSocket pour Groupie-Tracker
 */
class GameWebSocket {
  constructor(roomCode, handlers = {}) {
    this.roomCode = roomCode;
    this.handlers = handlers;
    this.ws = null;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 5;
    this.reconnectDelay = 1000;
    this.pingInterval = null;
    this.connected = false;
  }

  /**
   * Connecte au serveur WebSocket
   */
  connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${window.location.host}/ws?room=${this.roomCode}`;
    
    console.log('🔌 Connexion WebSocket:', url);
    
    this.ws = new WebSocket(url);
    
    this.ws.onopen = () => {
      console.log('✅ WebSocket connecté');
      this.connected = true;
      this.reconnectAttempts = 0;
      this.startPingInterval();
      
      if (this.handlers.onConnect) {
        this.handlers.onConnect();
      }
    };
    
    this.ws.onmessage = (event) => {
      try {
        // Peut contenir plusieurs messages séparés par \n
        const messages = event.data.split('\n').filter(m => m.trim());
        messages.forEach(msgStr => {
          const msg = JSON.parse(msgStr);
          this.handleMessage(msg);
        });
      } catch (e) {
        console.error('❌ Erreur parsing message:', e);
      }
    };
    
    this.ws.onclose = (event) => {
      console.log('🔌 WebSocket déconnecté', event.code, event.reason);
      this.connected = false;
      this.stopPingInterval();
      
      if (this.handlers.onDisconnect) {
        this.handlers.onDisconnect();
      }
      
      // Reconnexion automatique
      if (this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnectAttempts++;
        console.log(`🔄 Tentative de reconnexion ${this.reconnectAttempts}/${this.maxReconnectAttempts}...`);
        setTimeout(() => this.connect(), this.reconnectDelay * this.reconnectAttempts);
      }
    };
    
    this.ws.onerror = (error) => {
      console.error('❌ Erreur WebSocket:', error);
      if (this.handlers.onError) {
        this.handlers.onError(error);
      }
    };
  }

  /**
   * Déconnecte du serveur
   */
  disconnect() {
    this.stopPingInterval();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  /**
   * Envoie un message
   */
  send(type, payload = null) {
    if (!this.connected) {
      console.warn('⚠️ WebSocket non connecté');
      return false;
    }
    
    const msg = { type };
    if (payload !== null) {
      msg.payload = payload;
    }
    
    this.ws.send(JSON.stringify(msg));
    return true;
  }

  /**
   * Gère les messages reçus
   */
  handleMessage(msg) {
    console.log('📩 Message reçu:', msg.type, msg.payload);
    
    // Handler d'erreur global
    if (msg.type === 'error') {
      console.error('❌ Erreur serveur:', msg.error);
      if (this.handlers.onError) {
        this.handlers.onError(new Error(msg.error));
      }
      return;
    }
    
    // Pong
    if (msg.type === 'pong') {
      return;
    }
    
    // Handler spécifique par type
    const handlerName = this.getHandlerName(msg.type);
    if (this.handlers[handlerName]) {
      this.handlers[handlerName](msg.payload);
    }
    
    // Handler générique
    if (this.handlers.onMessage) {
      this.handlers.onMessage(msg);
    }
  }

  /**
   * Convertit un type de message en nom de handler
   */
  getHandlerName(type) {
    // bt_new_round -> onBtNewRound
    return 'on' + type.split('_').map(word => 
      word.charAt(0).toUpperCase() + word.slice(1)
    ).join('');
  }

  /**
   * Démarre l'intervalle de ping
   */
  startPingInterval() {
    this.pingInterval = setInterval(() => {
      this.send('ping');
    }, 30000);
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

  // === Méthodes de commodité ===

  /**
   * Signale qu'on est prêt
   */
  setReady(ready = true) {
    return this.send('player_ready', { ready });
  }

  /**
   * Quitte la salle
   */
  leaveRoom() {
    return this.send('leave_room');
  }

  /**
   * Démarre la partie (hôte uniquement)
   */
  startGame() {
    return this.send('start_game');
  }

  // === Blind Test ===

  /**
   * Envoie une réponse Blind Test
   */
  submitBlindTestAnswer(answer) {
    return this.send('bt_answer', { answer });
  }

  // === Petit Bac ===

  /**
   * Envoie les réponses Petit Bac
   */
  submitPetitBacAnswers(answers) {
    return this.send('pb_answer', { answers });
  }

  /**
   * Stoppe la manche Petit Bac
   */
  stopPetitBacRound() {
    return this.send('pb_stop_round');
  }

  /**
   * Vote pour une réponse Petit Bac
   */
  submitPetitBacVote(targetUserId, category, isValid) {
    return this.send('pb_vote', {
      target_user_id: targetUserId,
      category: category,
      is_valid: isValid
    });
  }
}

// Export global
window.GameWebSocket = GameWebSocket;