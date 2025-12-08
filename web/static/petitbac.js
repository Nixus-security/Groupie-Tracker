/**
 * Interface du jeu Petit Bac Musical
 */
class PetitBacGame {
  constructor(roomCode) {
    this.roomCode = roomCode;
    this.ws = null;
    this.currentRound = 0;
    this.totalRounds = 9;
    this.currentLetter = '';
    this.categories = [];
    this.scores = {};
    this.hasSubmitted = false;
    this.isVoting = false;
    this.votesToDo = [];
    
    this.init();
  }

  /**
   * Initialise le jeu
   */
  init() {
    // Éléments DOM
    this.elements = {
      roundNumber: document.getElementById('round-number'),
      currentLetter: document.getElementById('current-letter'),
      categoriesForm: document.getElementById('categories-form'),
      submitBtn: document.getElementById('submit-answers'),
      stopBtn: document.getElementById('stop-round'),
      votingSection: document.getElementById('voting-section'),
      playersScores: document.getElementById('players-scores')
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
      
      // Messages Petit Bac
      onPbNewRound: (data) => this.onNewRound(data),
      onPbAnswer: (data) => this.onAnswer(data),
      onPbStopRound: (data) => this.onStopRound(data),
      onPbVote: (data) => this.onVote(data),
      onPbVoteResult: (data) => this.onVoteResult(data),
      onPbScores: (data) => this.onScores(data),
      onPbGameEnd: (data) => this.onGameEnd(data)
    });

    this.ws.connect();

    // Événements
    this.elements.submitBtn.addEventListener('click', () => this.submitAnswers());
    this.elements.stopBtn.addEventListener('click', () => this.stopRound());
  }

  // === Événements WebSocket ===

  onConnect() {
    console.log('✅ Connecté au Petit Bac');
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
    this.players = data.players || [];
    this.updatePlayersScores();
  }

  onPlayerJoined(data) {
    console.log('👋 Joueur rejoint:', data.pseudo);
    this.showMessage(`${data.pseudo} a rejoint la partie`, 'info');
  }

  onPlayerLeft(data) {
    console.log('👋 Joueur parti:', data.pseudo);
    this.showMessage(`${data.pseudo} a quitté la partie`, 'warning');
  }

  /**
   * Nouvelle manche
   */
  onNewRound(data) {
    console.log('🎼 Nouvelle manche:', data);
    
    this.currentRound = data.round;
    this.totalRounds = data.total_rounds;
    this.currentLetter = data.letter;
    this.categories = data.categories || [];
    this.hasSubmitted = false;
    this.isVoting = false;
    
    // Mise à jour UI
    this.elements.roundNumber.textContent = `Manche ${this.currentRound}/${this.totalRounds}`;
    this.elements.currentLetter.textContent = this.currentLetter;
    this.elements.currentLetter.classList.add('bounce');
    setTimeout(() => this.elements.currentLetter.classList.remove('bounce'), 500);
    
    // Créer le formulaire
    this.createCategoriesForm();
    
    // Activer les boutons
    this.elements.submitBtn.disabled = false;
    this.elements.stopBtn.disabled = true; // Actif seulement après avoir soumis
    this.elements.votingSection.style.display = 'none';
    
    // Animation
    document.getElementById('game-container').classList.add('fade-in');
  }

  /**
   * Réponse d'un joueur
   */
  onAnswer(data) {
    console.log('📝 Réponse:', data);
    this.showMessage(`${data.pseudo} a soumis ses réponses`, 'info');
  }

  /**
   * Manche stoppée
   */
  onStopRound(data) {
    console.log('🛑 Manche stoppée par:', data.pseudo);
    this.showMessage(`${data.pseudo} a stoppé la manche !`, 'warning');
    
    // Désactiver les inputs si pas déjà soumis
    if (!this.hasSubmitted) {
      this.submitAnswers();
    }
  }

  /**
   * Phase de vote
   */
  onVote(data) {
    console.log('🗳️ Vote:', data);
    
    if (data.phase === 'start') {
      this.isVoting = true;
      this.votesToDo = [];
      this.displayVotingInterface(data.answers);
    } else if (data.phase === 'vote') {
      // Un joueur a voté
      this.showMessage('Vote enregistré', 'info');
    }
  }

  /**
   * Résultats des votes
   */
  onVoteResult(data) {
    console.log('📊 Résultats votes:', data);
    
    this.isVoting = false;
    this.scores = data.scores || {};
    
    // Afficher les résultats
    this.displayResults(data.results);
    this.updatePlayersScores();
  }

  /**
   * Mise à jour des scores
   */
  onScores(data) {
    console.log('🏆 Scores:', data);
    this.scores = data;
    this.updatePlayersScores();
  }

  /**
   * Fin de partie
   */
  onGameEnd(data) {
    console.log('🏁 Fin de partie:', data);
    
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
   * Crée le formulaire des catégories
   */
  createCategoriesForm() {
    let html = '';
    
    this.categories.forEach(category => {
      html += `
        <div class="category-input">
          <label for="cat-${category}">${category}</label>
          <input type="text" 
                 id="cat-${category}" 
                 name="${category}" 
                 placeholder="${this.currentLetter}..."
                 autocomplete="off"
                 data-category="${category}">
        </div>
      `;
    });
    
    this.elements.categoriesForm.innerHTML = html;
    
    // Focus sur le premier input
    const firstInput = this.elements.categoriesForm.querySelector('input');
    if (firstInput) firstInput.focus();
    
    // Validation en temps réel
    const inputs = this.elements.categoriesForm.querySelectorAll('input');
    inputs.forEach(input => {
      input.addEventListener('input', (e) => {
        const value = e.target.value.toUpperCase();
        if (value && !value.startsWith(this.currentLetter)) {
          e.target.classList.add('shake');
          setTimeout(() => e.target.classList.remove('shake'), 300);
        }
      });
      
      // Passage au suivant avec Enter
      input.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          const inputs = Array.from(this.elements.categoriesForm.querySelectorAll('input'));
          const currentIndex = inputs.indexOf(e.target);
          if (currentIndex < inputs.length - 1) {
            inputs[currentIndex + 1].focus();
          } else {
            this.submitAnswers();
          }
        }
      });
    });
  }

  /**
   * Soumet les réponses
   */
  submitAnswers() {
    if (this.hasSubmitted) return;
    
    const answers = {};
    const inputs = this.elements.categoriesForm.querySelectorAll('input');
    
    inputs.forEach(input => {
      const category = input.dataset.category;
      answers[category] = input.value.trim();
      input.disabled = true;
    });
    
    this.ws.submitPetitBacAnswers(answers);
    this.hasSubmitted = true;
    
    this.elements.submitBtn.disabled = true;
    this.elements.stopBtn.disabled = false;
    
    this.showMessage('Réponses envoyées !', 'success');
  }

  /**
   * Stoppe la manche
   */
  stopRound() {
    if (!this.hasSubmitted) {
      this.submitAnswers();
    }
    this.ws.stopPetitBacRound();
    this.elements.stopBtn.disabled = true;
  }

  /**
   * Affiche l'interface de vote
   */
  displayVotingInterface(answersData) {
    let html = '<h3>🗳️ Phase de vote</h3>';
    html += '<p>Validez ou contestez les réponses des autres joueurs</p>';
    
    // Récupérer l'ID de l'utilisateur actuel (depuis une variable globale ou cookie)
    const currentUserId = this.getCurrentUserId();
    
    for (const category in answersData) {
      const answers = answersData[category];
      if (answers.length === 0) continue;
      
      html += `<div class="vote-category">`;
      html += `<h4 class="mt-2">${category}</h4>`;
      
      answers.forEach(a => {
        // Ne pas voter pour ses propres réponses
        if (a.user_id === currentUserId) {
          html += `
            <div class="vote-item my-answer">
              <span>${a.pseudo}: <strong>${a.answer}</strong></span>
              <span class="text-muted">(votre réponse)</span>
            </div>
          `;
        } else {
          html += `
            <div class="vote-item" data-user-id="${a.user_id}" data-category="${category}">
              <span>${a.pseudo}: <strong>${a.answer}</strong></span>
              <div class="vote-buttons">
                <button class="vote-btn valid" onclick="petitBacGame.vote(${a.user_id}, '${category}', true)">✓ Valide</button>
                <button class="vote-btn invalid" onclick="petitBacGame.vote(${a.user_id}, '${category}', false)">✗ Invalide</button>
              </div>
            </div>
          `;
          
          this.votesToDo.push({ userId: a.user_id, category });
        }
      });
      
      html += `</div>`;
    }
    
    if (this.votesToDo.length === 0) {
      html += '<p class="text-center mt-2">Aucun vote nécessaire, attente des autres joueurs...</p>';
    }
    
    this.elements.votingSection.innerHTML = html;
    this.elements.votingSection.style.display = 'block';
    this.elements.categoriesForm.style.display = 'none';
    document.getElementById('actions').style.display = 'none';
  }

  /**
   * Vote pour une réponse
   */
  vote(targetUserId, category, isValid) {
    this.ws.submitPetitBacVote(targetUserId, category, isValid);
    
    // Désactiver les boutons de ce vote
    const item = this.elements.votingSection.querySelector(
      `.vote-item[data-user-id="${targetUserId}"][data-category="${category}"]`
    );
    if (item) {
      const buttons = item.querySelectorAll('.vote-btn');
      buttons.forEach(btn => btn.disabled = true);
      item.classList.add(isValid ? 'voted-valid' : 'voted-invalid');
    }
  }

  /**
   * Affiche les résultats de la manche
   */
  displayResults(results) {
    let html = '<h3>📊 Résultats de la manche</h3>';
    
    for (const category in results) {
      html += `<div class="result-category">`;
      html += `<h4>${category}</h4>`;
      html += `<table class="results-table">`;
      html += `<thead><tr><th>Joueur</th><th>Réponse</th><th>Votes</th><th>Points</th></tr></thead>`;
      html += `<tbody>`;
      
      results[category].forEach(r => {
        const validClass = r.is_valid ? 'valid' : 'invalid';
        html += `
          <tr class="${validClass}">
            <td>${r.pseudo}</td>
            <td>${r.answer || '-'}</td>
            <td>${r.votes_for}👍 / ${r.votes_against}👎</td>
            <td>+${r.points}</td>
          </tr>
        `;
      });
      
      html += `</tbody></table></div>`;
    }
    
    this.elements.votingSection.innerHTML = html;
    this.elements.votingSection.style.display = 'block';
  }

  // === UI Helpers ===

  getCurrentUserId() {
    // À adapter selon votre système d'authentification
    // Option 1: Variable globale
    if (window.currentUserId) return window.currentUserId;
    
    // Option 2: Depuis les données de la page
    const userEl = document.querySelector('[data-current-user-id]');
    if (userEl) return parseInt(userEl.dataset.currentUserId);
    
    return null;
  }

  updatePlayersScores() {
    if (!this.players) return;
    
    let html = '<h3>Scores</h3>';
    
    // Trier par score
    const sortedPlayers = [...this.players].sort((a, b) => {
      const scoreA = this.scores[a.user_id] || 0;
      const scoreB = this.scores[b.user_id] || 0;
      return scoreB - scoreA;
    });
    
    sortedPlayers.forEach((p, index) => {
      const score = this.scores[p.user_id] || 0;
      html += `
        <div class="player-item ${index === 0 ? 'first' : ''}">
          <div class="player-info">
            <div class="player-avatar">${p.pseudo.charAt(0).toUpperCase()}</div>
            <span>${p.pseudo}</span>
          </div>
          <div class="player-score">${score} pts</div>
        </div>
      `;
    });
    
    this.elements.playersScores.innerHTML = html;
  }

  getPlayerPseudo(userId) {
    if (!this.players) return `Joueur ${userId}`;
    const player = this.players.find(p => p.user_id === userId);
    return player ? player.pseudo : `Joueur ${userId}`;
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
  const pathParts = window.location.pathname.split('/');
  const roomCode = pathParts[pathParts.length - 1] || new URLSearchParams(window.location.search).get('room');
  
  if (roomCode) {
    window.petitBacGame = new PetitBacGame(roomCode);
  }
});