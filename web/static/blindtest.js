/**
 * Interface du jeu Blind Test
 */
class BlindTestGame {
  constructor(roomCode) {
    this.roomCode = roomCode;
    this.ws = null;
    this.currentRound = 0;
    this.totalRounds = 10;
    this.timeLeft = 37;
    this.timerInterval = null;
    this.hasAnswered = false;
    this.scores = {};
    
    this.init();
  }

  /**
   * Initialise le jeu
   */
  init() {
    // Éléments DOM
    this.elements = {
      roundNumber: document.getElementById('round-number'),
      timer: document.getElementById('timer'),
      audio: document.getElementById('preview-audio'),
      answerInput: document.getElementById('answer-input'),
      submitBtn: document.getElementById('submit-answer'),
      playersList: document.getElementById('players-list'),
      results: document.getElementById('results')
    };

    // Connexion WebSocket
    this.ws = new GameWebSocket(this.roomCode, {
      onConnect: () => this.onConnect(),
      onDisconnect: () => this.onDisconnect(),
      onError: (err) => this.onError(err),
      
      // Messages de salle
      onRoomUpdate: (data) => this.onRoomUpdate(data),
      onPlayerJoined: (data) => this.onPlayerJoined(data),
      onPlayerLeft: (data) => this.onPlayerLeft(data),
      onPlayerReady: (data) => this.onPlayerReady(data),
      
      // Messages Blind Test
      onBtNewRound: (data) => this.onNewRound(data),
      onBtAnswer: (data) => this.onAnswer(data),
      onBtResult: (data) => this.onResult(data),
      onBtScores: (data) => this.onScores(data),
      onBtGameEnd: (data) => this.onGameEnd(data)
    });

    this.ws.connect();

    // Événements
    this.elements.submitBtn.addEventListener('click', () => this.submitAnswer());
    this.elements.answerInput.addEventListener('keypress', (e) => {
      if (e.key === 'Enter') this.submitAnswer();
    });
  }

  // === Événements WebSocket ===

  onConnect() {
    console.log('✅ Connecté au Blind Test');
    this.showMessage('Connecté !', 'success');
  }

  onDisconnect() {
    console.log('❌ Déconnecté');
    this.showMessage('Déconnecté du serveur...', 'danger');
  }

  onError(err) {
    console.error('Erreur:', err);
    this.showMessage(err.message || 'Erreur de connexion', 'danger');
  }

  onRoomUpdate(data) {
    console.log('🏠 Mise à jour salle:', data);
    this.updatePlayersList(data.players || []);
  }

  onPlayerJoined(data) {
    console.log('👋 Joueur rejoint:', data.pseudo);
    this.showMessage(`${data.pseudo} a rejoint la partie`, 'info');
  }

  onPlayerLeft(data) {
    console.log('👋 Joueur parti:', data.pseudo);
    this.showMessage(`${data.pseudo} a quitté la partie`, 'warning');
  }

  onPlayerReady(data) {
    console.log('✓ Joueur prêt:', data.pseudo, data.ready);
  }

  /**
   * Nouvelle manche
   */
  onNewRound(data) {
    console.log('🎵 Nouvelle manche:', data);
    
    this.currentRound = data.round;
    this.totalRounds = data.total_rounds;
    this.timeLeft = data.duration || 37;
    this.hasAnswered = false;
    
    // Mise à jour UI
    this.elements.roundNumber.textContent = `Manche ${this.currentRound}/${this.totalRounds}`;
    this.elements.answerInput.value = '';
    this.elements.answerInput.disabled = false;
    this.elements.submitBtn.disabled = false;
    this.elements.results.style.display = 'none';
    
    // Jouer l'audio
    if (data.preview_url) {
      this.elements.audio.src = data.preview_url;
      this.elements.audio.play().catch(e => {
        console.warn('Autoplay bloqué:', e);
        this.showMessage('Cliquez pour lancer l\'audio', 'warning');
      });
    }
    
    // Démarrer le timer
    this.startTimer();
    
    // Animation
    document.getElementById('game-container').classList.add('fade-in');
  }

  /**
   * Réponse d'un joueur
   */
  onAnswer(data) {
    console.log('🎤 Réponse:', data);
    
    if (data.correct) {
      this.showMessage(`${data.pseudo} a trouvé !`, 'success');
      
      // Mettre en évidence le joueur
      const playerEl = document.querySelector(`[data-user-id="${data.user_id}"]`);
      if (playerEl) {
        playerEl.classList.add('bounce');
        setTimeout(() => playerEl.classList.remove('bounce'), 500);
      }
    }
  }

  /**
   * Résultats de la manche
   */
  onResult(data) {
    console.log('📊 Résultats:', data);
    
    // Arrêter le timer et l'audio
    this.stopTimer();
    this.elements.audio.pause();
    
    // Désactiver les inputs
    this.elements.answerInput.disabled = true;
    this.elements.submitBtn.disabled = true;
    
    // Afficher la réponse
    const track = data.track;
    this.elements.results.innerHTML = `
      <div class="track-reveal fade-in">
        <img src="${track.image_url || '/static/images/album-placeholder.png'}" alt="${track.name}">
        <h3>${track.name}</h3>
        <p>${track.artist}</p>
      </div>
      <table class="results-table mt-2">
        <thead>
          <tr>
            <th>Joueur</th>
            <th>Réponse</th>
            <th>Points</th>
          </tr>
        </thead>
        <tbody>
          ${(data.results || []).map(r => `
            <tr class="${r.correct ? 'correct' : 'wrong'}">
              <td>${r.pseudo}</td>
              <td>${r.answer || '-'}</td>
              <td>${r.correct ? `+${r.points}` : '0'}</td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    `;
    this.elements.results.style.display = 'block';
    
    // Mettre à jour les scores
    this.scores = data.scores || {};
    this.updateScores();
  }

  /**
   * Mise à jour des scores
   */
  onScores(data) {
    console.log('🏆 Scores:', data);
    this.scores = data;
    this.updateScores();
  }

  /**
   * Fin de partie
   */
  onGameEnd(data) {
    console.log('🏁 Fin de partie:', data);
    
    this.stopTimer();
    this.elements.audio.pause();
    
    // Afficher le classement final
    const rankings = data.rankings || [];
    const html = `
      <div class="final-ranking fade-in">
        <h2>🏆 Partie terminée !</h2>
        <div class="winner">
          ${rankings[0] ? `👑 ${this.getPlayerPseudo(rankings[0].user_id)}` : ''}
        </div>
        <div class="podium">
          ${rankings.slice(0, 3).map((r, i) => `
            <div class="podium-item ${['first', 'second', 'third'][i]}">
              <div class="rank">${i + 1}</div>
              <div class="pseudo">${this.getPlayerPseudo(r.user_id)}</div>
              <div class="score">${r.score} pts</div>
            </div>
          `).join('')}
        </div>
        <table class="results-table mt-4">
          <thead>
            <tr><th>#</th><th>Joueur</th><th>Score</th></tr>
          </thead>
          <tbody>
            ${rankings.map(r => `
              <tr class="rank-${r.rank}">
                <td>${r.rank}</td>
                <td>${this.getPlayerPseudo(r.user_id)}</td>
                <td>${r.score} pts</td>
              </tr>
            `).join('')}
          </tbody>
        </table>
        <div class="mt-4">
          <a href="/rooms" class="btn btn-primary">Retour aux salles</a>
        </div>
      </div>
    `;
    
    document.getElementById('game-container').innerHTML = html;
  }

  // === Actions ===

  /**
   * Soumet une réponse
   */
  submitAnswer() {
    if (this.hasAnswered) return;
    
    const answer = this.elements.answerInput.value.trim();
    if (!answer) return;
    
    this.ws.submitBlindTestAnswer(answer);
    this.hasAnswered = true;
    this.elements.answerInput.disabled = true;
    this.elements.submitBtn.disabled = true;
    
    this.showMessage('Réponse envoyée !', 'info');
  }

  // === Timer ===

  startTimer() {
    this.stopTimer();
    this.updateTimerDisplay();
    
    this.timerInterval = setInterval(() => {
      this.timeLeft--;
      this.updateTimerDisplay();
      
      if (this.timeLeft <= 0) {
        this.stopTimer();
      }
    }, 1000);
  }

  stopTimer() {
    if (this.timerInterval) {
      clearInterval(this.timerInterval);
      this.timerInterval = null;
    }
  }

  updateTimerDisplay() {
    this.elements.timer.textContent = `${this.timeLeft}s`;
    
    if (this.timeLeft <= 10) {
      this.elements.timer.classList.add('danger');
    } else {
      this.elements.timer.classList.remove('danger');
    }
  }

  // === UI Helpers ===

  updatePlayersList(players) {
    this.players = players;
    
    let html = '<h3>Joueurs</h3>';
    players.forEach(p => {
      const score = this.scores[p.user_id] || 0;
      html += `
        <div class="player-item ${p.is_host ? 'host' : ''}" data-user-id="${p.user_id}">
          <div class="player-info">
            <div class="player-avatar">${p.pseudo.charAt(0).toUpperCase()}</div>
            <div>
              <div>${p.pseudo}</div>
              <div class="player-status">
                ${p.is_host ? '👑 Hôte' : ''}
                ${p.connected ? '🟢' : '🔴'}
              </div>
            </div>
          </div>
          <div class="player-score">${score} pts</div>
        </div>
      `;
    });
    
    this.elements.playersList.innerHTML = html;
  }

  updateScores() {
    const playerItems = this.elements.playersList.querySelectorAll('.player-item');
    playerItems.forEach(item => {
      const userId = parseInt(item.dataset.userId);
      const score = this.scores[userId] || 0;
      const scoreEl = item.querySelector('.player-score');
      if (scoreEl) {
        scoreEl.textContent = `${score} pts`;
      }
    });
  }

  getPlayerPseudo(userId) {
    if (!this.players) return `Joueur ${userId}`;
    const player = this.players.find(p => p.user_id === userId);
    return player ? player.pseudo : `Joueur ${userId}`;
  }

  showMessage(message, type = 'info') {
    // Créer une notification temporaire
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
  // Récupérer le code de salle depuis l'URL
  const pathParts = window.location.pathname.split('/');
  const roomCode = pathParts[pathParts.length - 1] || new URLSearchParams(window.location.search).get('room');
  
  if (roomCode) {
    window.blindTestGame = new BlindTestGame(roomCode);
  }
});