/**
 * ============================================================================
 * GROUPIE-TRACKER - Blind Test Game
 * ============================================================================
 */

class BlindTestGame {
    constructor() {
        this.currentRound = 0;
        this.totalRounds = 10;
        this.timePerRound = 37;
        this.timeRemaining = 0;
        this.timerInterval = null;
        this.isPlaying = false;
        this.hasAnswered = false;
        this.scores = {};
        this.currentTrack = null;
        this.audioElement = null;

        this.init();
    }

    /**
     * Initialise le jeu
     */
    init() {
        this.setupElements();
        this.setupEventListeners();
        this.setupWebSocketHandlers();
    }

    /**
     * Configure les éléments DOM
     */
    setupElements() {
        this.elements = {
            roundNumber: document.getElementById('round-number'),
            timer: document.getElementById('timer'),
            audioPlayer: document.getElementById('preview-audio'),
            answerInput: document.getElementById('answer-input'),
            submitBtn: document.getElementById('submit-answer'),
            playersList: document.getElementById('players-list'),
            results: document.getElementById('results'),
            gameContainer: document.getElementById('game-container'),
            trackImage: document.querySelector('.track-image')
        };

        this.audioElement = this.elements.audioPlayer;
    }

    /**
     * Configure les listeners d'événements
     */
    setupEventListeners() {
        // Soumission de réponse
        if (this.elements.submitBtn) {
            this.elements.submitBtn.addEventListener('click', () => this.submitAnswer());
        }

        // Soumission avec Entrée
        if (this.elements.answerInput) {
            this.elements.answerInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') {
                    this.submitAnswer();
                }
            });

            // Debounce sur l'input
            this.elements.answerInput.addEventListener('input', debounce(() => {
                // Optionnel: validation en temps réel
            }, 200));
        }
    }

    /**
     * Configure les handlers WebSocket
     */
    setupWebSocketHandlers() {
        // Nouvelle manche
        wsManager.on('bt_new_round', (data) => {
            this.handleNewRound(data);
        });

        // Réponse d'un joueur
        wsManager.on('bt_answer', (data) => {
            this.handlePlayerAnswer(data);
        });

        // Résultats de la manche
        wsManager.on('bt_result', (data) => {
            this.handleRoundResult(data);
        });

        // Fin de partie
        wsManager.on('bt_game_end', (data) => {
            this.handleGameEnd(data);
        });

        // Mise à jour des scores
        wsManager.on('bt_scores', (data) => {
            this.updateScores(data);
        });
    }

    /**
     * Gère le début d'une nouvelle manche
     * @param {Object} data - Données de la manche
     */
    handleNewRound(data) {
        console.log('🎵 Nouvelle manche:', data);

        this.currentRound = data.round;
        this.totalRounds = data.total_rounds;
        this.timeRemaining = data.duration || this.timePerRound;
        this.hasAnswered = false;
        this.isPlaying = true;
        this.currentTrack = null;

        // Mettre à jour l'UI
        this.updateRoundInfo();
        this.resetAnswerForm();
        this.hideResults();

        // Charger et jouer l'audio
        if (data.preview_url) {
            this.playAudio(data.preview_url);
        }

        // Afficher l'image floue si disponible
        if (data.image_url && this.elements.trackImage) {
            this.elements.trackImage.src = data.image_url;
            this.elements.trackImage.classList.add('blur');
        }

        // Démarrer le timer
        this.startTimer();

        // Animation
        this.animateNewRound();

        showToast(`Manche ${this.currentRound}/${this.totalRounds}`, 'info');
    }

    /**
     * Met à jour les informations de manche
     */
    updateRoundInfo() {
        if (this.elements.roundNumber) {
            this.elements.roundNumber.textContent = `Manche ${this.currentRound}/${this.totalRounds}`;
        }
    }

    /**
     * Démarre le timer
     */
    startTimer() {
        this.stopTimer();

        this.updateTimerDisplay();

        this.timerInterval = setInterval(() => {
            this.timeRemaining--;
            this.updateTimerDisplay();

            if (this.timeRemaining <= 0) {
                this.stopTimer();
            }
        }, 1000);
    }

    /**
     * Arrête le timer
     */
    stopTimer() {
        if (this.timerInterval) {
            clearInterval(this.timerInterval);
            this.timerInterval = null;
        }
    }

    /**
     * Met à jour l'affichage du timer
     */
    updateTimerDisplay() {
        if (this.elements.timer) {
            this.elements.timer.textContent = `${this.timeRemaining}s`;

            // Classes de warning
            this.elements.timer.classList.remove('warning', 'danger');
            if (this.timeRemaining <= 10) {
                this.elements.timer.classList.add('danger');
            } else if (this.timeRemaining <= 20) {
                this.elements.timer.classList.add('warning');
            }
        }
    }

    /**
     * Joue l'audio de preview
     * @param {string} url - URL de l'audio
     */
    playAudio(url) {
        if (this.audioElement) {
            this.audioElement.src = url;
            this.audioElement.currentTime = 0;
            
            // Tentative de lecture automatique
            const playPromise = this.audioElement.play();
            
            if (playPromise !== undefined) {
                playPromise.catch(error => {
                    console.warn('⚠️ Autoplay bloqué:', error);
                    showToast('Cliquez sur le lecteur pour écouter', 'warning');
                });
            }
        }
    }

    /**
     * Arrête l'audio
     */
    stopAudio() {
        if (this.audioElement) {
            this.audioElement.pause();
            this.audioElement.currentTime = 0;
        }
    }

    /**
     * Soumet la réponse du joueur
     */
    submitAnswer() {
        if (this.hasAnswered || !this.isPlaying) {
            return;
        }

        const answer = this.elements.answerInput?.value.trim();
        
        if (!answer) {
            showToast('Entrez une réponse', 'warning');
            animateElement(this.elements.answerInput, 'animate-shake');
            return;
        }

        this.hasAnswered = true;
        
        // Envoyer via WebSocket
        wsManager.submitBlindTestAnswer(answer);

        // Désactiver le formulaire
        this.disableAnswerForm();

        showToast('Réponse envoyée !', 'success');
    }

    /**
     * Réinitialise le formulaire de réponse
     */
    resetAnswerForm() {
        if (this.elements.answerInput) {
            this.elements.answerInput.value = '';
            this.elements.answerInput.disabled = false;
            this.elements.answerInput.focus();
        }
        if (this.elements.submitBtn) {
            this.elements.submitBtn.disabled = false;
        }
    }

    /**
     * Désactive le formulaire de réponse
     */
    disableAnswerForm() {
        if (this.elements.answerInput) {
            this.elements.answerInput.disabled = true;
        }
        if (this.elements.submitBtn) {
            this.elements.submitBtn.disabled = true;
        }
    }

    /**
     * Gère la réponse d'un joueur
     * @param {Object} data - Données de la réponse
     */
    handlePlayerAnswer(data) {
        if (data.correct) {
            showToast(`${data.pseudo} a trouvé !`, 'success');
            this.animateCorrectAnswer(data.user_id);
        }
    }

    /**
     * Gère les résultats de la manche
     * @param {Object} data - Résultats
     */
    handleRoundResult(data) {
        console.log('📊 Résultats manche:', data);

        this.isPlaying = false;
        this.stopTimer();
        this.stopAudio();

        // Afficher le titre révélé
        if (data.track) {
            this.revealTrack(data.track);
        }

        // Afficher les résultats
        this.showResults(data.results);

        // Mettre à jour les scores
        if (data.scores) {
            this.updateScores(data.scores);
        }

        // Enlever le flou de l'image
        if (this.elements.trackImage) {
            this.elements.trackImage.classList.remove('blur');
        }
    }

    /**
     * Révèle le titre de la chanson
     * @param {Object} track - Informations du titre
     */
    revealTrack(track) {
        this.currentTrack = track;

        // Créer ou mettre à jour l'élément de révélation
        let revealElement = document.querySelector('.track-reveal');
        
        if (!revealElement) {
            revealElement = document.createElement('div');
            revealElement.className = 'track-reveal';
            this.elements.gameContainer?.insertBefore(
                revealElement,
                this.elements.results
            );
        }

        revealElement.innerHTML = `
            <h3>🎵 ${track.name}</h3>
            <p>par ${track.artist}</p>
            ${track.album ? `<p class="text-muted">${track.album}</p>` : ''}
        `;

        revealElement.style.display = 'block';
        animateElement(revealElement, 'animate-bounce');
    }

    /**
     * Affiche les résultats de la manche
     * @param {Array} results - Liste des résultats
     */
    showResults(results) {
        if (!this.elements.results) return;

        let html = '<h3>📊 Résultats de la manche</h3>';
        
        if (results && results.length > 0) {
            html += '<div class="results-list">';
            
            // Trier par points décroissants
            results.sort((a, b) => b.points - a.points);
            
            results.forEach(result => {
                const statusClass = result.correct ? 'correct' : 'wrong';
                html += `
                    <div class="result-item ${statusClass}">
                        <div class="result-player">
                            <span class="player-name">${result.pseudo}</span>
                            ${result.answer ? `<span class="player-answer">"${result.answer}"</span>` : '<span class="text-muted">Pas de réponse</span>'}
                        </div>
                        <div class="result-points">
                            ${result.correct ? `+${result.points} pts` : '0 pt'}
                        </div>
                    </div>
                `;
            });
            
            html += '</div>';
        } else {
            html += '<p class="text-muted">Personne n\'a trouvé cette fois !</p>';
        }

        this.elements.results.innerHTML = html;
        this.elements.results.style.display = 'block';
        animateElement(this.elements.results, 'animate-slide-up');
    }

    /**
     * Cache les résultats
     */
    hideResults() {
        if (this.elements.results) {
            this.elements.results.style.display = 'none';
        }

        // Cacher aussi la révélation du titre
        const revealElement = document.querySelector('.track-reveal');
        if (revealElement) {
            revealElement.style.display = 'none';
        }
    }

    /**
     * Met à jour les scores
     * @param {Object} scores - Scores des joueurs
     */
    updateScores(scores) {
        this.scores = scores;
        this.renderScoreboard();
    }

    /**
     * Affiche le tableau des scores
     */
    renderScoreboard() {
        if (!this.elements.playersList) return;

        // Convertir en tableau et trier
        const entries = Object.entries(this.scores).map(([id, score]) => ({
            userId: parseInt(id),
            score: score
        }));
        
        entries.sort((a, b) => b.score - a.score);

        let html = '<h3>🏆 Scores</h3><div class="scoreboard">';
        
        entries.forEach((entry, index) => {
            const rankClass = index < 3 ? `rank-${index + 1}` : '';
            const rankEmoji = index === 0 ? '🥇' : index === 1 ? '🥈' : index === 2 ? '🥉' : `${index + 1}.`;
            
            // Trouver le pseudo du joueur (à améliorer avec les données réelles)
            const playerCard = document.querySelector(`[data-user-id="${entry.userId}"]`);
            const pseudo = playerCard?.querySelector('.player-name')?.textContent || `Joueur ${entry.userId}`;
            
            html += `
                <div class="score-row ${rankClass}">
                    <span class="score-rank">${rankEmoji}</span>
                    <span class="score-player">${pseudo}</span>
                    <span class="score-value">${entry.score} pts</span>
                </div>
            `;
        });

        html += '</div>';
        this.elements.playersList.innerHTML = html;
    }

    /**
     * Gère la fin de partie
     * @param {Object} data - Données de fin de partie
     */
    handleGameEnd(data) {
        console.log('🏆 Fin de partie:', data);

        this.isPlaying = false;
        this.stopTimer();
        this.stopAudio();

        showToast('🏆 Partie terminée !', 'success', 5000);

        // Afficher l'écran de fin
        this.showGameEndScreen(data);
    }

    /**
     * Affiche l'écran de fin de partie
     * @param {Object} data - Données finales
     */
    showGameEndScreen(data) {
        if (!this.elements.gameContainer) return;

        const rankings = data.rankings || [];
        const winner = rankings[0];

        let html = `
            <div class="game-end">
                <h1>🎉 Partie Terminée !</h1>
                
                ${winner ? `
                    <div class="winner-card">
                        <div class="trophy">🏆</div>
                        <div class="winner-name">Joueur #${winner.user_id}</div>
                        <div class="winner-score">${winner.score} points</div>
                    </div>
                ` : ''}
                
                <div class="final-rankings">
                    <h3>Classement Final</h3>
                    ${rankings.map((entry, index) => `
                        <div class="score-row ${index < 3 ? `rank-${index + 1}` : ''}">
                            <span class="score-rank">${index + 1}</span>
                            <span class="score-player">Joueur #${entry.user_id}</span>
                            <span class="score-value">${entry.score} pts</span>
                        </div>
                    `).join('')}
                </div>
                
                <div class="room-actions">
                    <a href="/rooms" class="btn btn-primary">Retour aux salles</a>
                    <button onclick="location.reload()" class="btn btn-secondary">Rejouer</button>
                </div>
            </div>
        `;

        this.elements.gameContainer.innerHTML = html;
    }

    /**
     * Animation pour une nouvelle manche
     */
    animateNewRound() {
        if (this.elements.roundNumber) {
            animateElement(this.elements.roundNumber, 'animate-bounce');
        }
    }

    /**
     * Animation pour une bonne réponse
     * @param {number} userId - ID du joueur
     */
    animateCorrectAnswer(userId) {
        const playerCard = document.querySelector(`[data-user-id="${userId}"]`);
        if (playerCard) {
            animateElement(playerCard, 'animate-bounce');
            playerCard.style.borderColor = 'var(--success)';
            setTimeout(() => {
                playerCard.style.borderColor = '';
            }, 2000);
        }
    }
}

// ============================================================================
// INITIALISATION
// ============================================================================

let blindTestGame;

/**
 * Initialise le jeu Blind Test
 * @param {Object} config - Configuration du jeu
 */
function initBlindTest(config = {}) {
    console.log('🎵 Initialisation Blind Test:', config);
    blindTestGame = new BlindTestGame();
    
    if (config.time_per_round) {
        blindTestGame.timePerRound = config.time_per_round;
    }
}

// Auto-initialisation si on est sur la page du Blind Test
document.addEventListener('DOMContentLoaded', () => {
    const gameContainer = document.getElementById('game-container');
    const isBlindTestPage = gameContainer && document.querySelector('#answer-input');
    
    if (isBlindTestPage) {
        initBlindTest();
    }
});

// Export
window.BlindTestGame = BlindTestGame;
window.initBlindTest = initBlindTest;