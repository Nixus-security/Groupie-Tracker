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

    init() {
        this.setupElements();
        this.setupEventListeners();
        this.setupWebSocketHandlers();
    }

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

    setupEventListeners() {
        if (this.elements.submitBtn) {
            this.elements.submitBtn.addEventListener('click', () => this.submitAnswer());
        }

        if (this.elements.answerInput) {
            this.elements.answerInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') {
                    this.submitAnswer();
                }
            });

            this.elements.answerInput.addEventListener('input', debounce(() => {
            }, 200));
        }
    }

    setupWebSocketHandlers() {
        wsManager.on('bt_new_round', (data) => {
            this.handleNewRound(data);
        });

        wsManager.on('bt_answer', (data) => {
            this.handlePlayerAnswer(data);
        });

        wsManager.on('bt_result', (data) => {
            this.handleRoundResult(data);
        });

        wsManager.on('bt_game_end', (data) => {
            this.handleGameEnd(data);
        });

        wsManager.on('bt_scores', (data) => {
            this.updateScores(data);
        });
    }

    handleNewRound(data) {
        console.log('🎵 Nouvelle manche:', data);

        this.currentRound = data.round;
        this.totalRounds = data.total_rounds;
        this.timeRemaining = data.duration || this.timePerRound;
        this.hasAnswered = false;
        this.isPlaying = true;
        this.currentTrack = null;

        this.updateRoundInfo();
        this.resetAnswerForm();
        this.hideResults();

        if (data.preview_url) {
            this.playAudio(data.preview_url);
        }

        if (data.image_url && this.elements.trackImage) {
            this.elements.trackImage.src = data.image_url;
            this.elements.trackImage.classList.add('blur');
        }

        this.startTimer();

        this.animateNewRound();

        showToast(`Manche ${this.currentRound}/${this.totalRounds}`, 'info');
    }

    updateRoundInfo() {
        if (this.elements.roundNumber) {
            this.elements.roundNumber.textContent = `Manche ${this.currentRound}/${this.totalRounds}`;
        }
    }

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

    stopTimer() {
        if (this.timerInterval) {
            clearInterval(this.timerInterval);
            this.timerInterval = null;
        }
    }

    updateTimerDisplay() {
        if (this.elements.timer) {
            this.elements.timer.textContent = `${this.timeRemaining}s`;

            this.elements.timer.classList.remove('warning', 'danger');
            if (this.timeRemaining <= 10) {
                this.elements.timer.classList.add('danger');
            } else if (this.timeRemaining <= 20) {
                this.elements.timer.classList.add('warning');
            }
        }
    }

    playAudio(url) {
        if (this.audioElement) {
            this.audioElement.src = url;
            this.audioElement.currentTime = 0;
            
            const playPromise = this.audioElement.play();
            
            if (playPromise !== undefined) {
                playPromise.catch(error => {
                    console.warn('⚠️ Autoplay bloqué:', error);
                    showToast('Cliquez sur le lecteur pour écouter', 'warning');
                });
            }
        }
    }

    stopAudio() {
        if (this.audioElement) {
            this.audioElement.pause();
            this.audioElement.currentTime = 0;
        }
    }

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
        
        wsManager.submitBlindTestAnswer(answer);

        this.disableAnswerForm();

        showToast('Réponse envoyée !', 'success');
    }

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

    disableAnswerForm() {
        if (this.elements.answerInput) {
            this.elements.answerInput.disabled = true;
        }
        if (this.elements.submitBtn) {
            this.elements.submitBtn.disabled = true;
        }
    }

    handlePlayerAnswer(data) {
        if (data.correct) {
            showToast(`${data.pseudo} a trouvé !`, 'success');
            this.animateCorrectAnswer(data.user_id);
        }
    }

    handleRoundResult(data) {
        console.log('📊 Résultats manche:', data);

        this.isPlaying = false;
        this.stopTimer();
        this.stopAudio();

        if (data.track) {
            this.revealTrack(data.track);
        }

        this.showResults(data.results);

        if (data.scores) {
            this.updateScores(data.scores);
        }

        if (this.elements.trackImage) {
            this.elements.trackImage.classList.remove('blur');
        }
    }

    revealTrack(track) {
        this.currentTrack = track;

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

    showResults(results) {
        if (!this.elements.results) return;

        let html = '<h3>📊 Résultats de la manche</h3>';
        
        if (results && results.length > 0) {
            html += '<div class="results-list">';
            
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

    hideResults() {
        if (this.elements.results) {
            this.elements.results.style.display = 'none';
        }

        const revealElement = document.querySelector('.track-reveal');
        if (revealElement) {
            revealElement.style.display = 'none';
        }
    }

    updateScores(scores) {
        this.scores = scores;
        this.renderScoreboard();
    }

    renderScoreboard() {
        if (!this.elements.playersList) return;

        const entries = Object.entries(this.scores).map(([id, score]) => ({
            userId: parseInt(id),
            score: score
        }));
        
        entries.sort((a, b) => b.score - a.score);

        let html = '<h3>🏆 Scores</h3><div class="scoreboard">';
        
        entries.forEach((entry, index) => {
            const rankClass = index < 3 ? `rank-${index + 1}` : '';
            const rankEmoji = index === 0 ? '🥇' : index === 1 ? '🥈' : index === 2 ? '🥉' : `${index + 1}.`;
            
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

    handleGameEnd(data) {
        console.log('🏆 Fin de partie:', data);

        this.isPlaying = false;
        this.stopTimer();
        this.stopAudio();

        showToast('🏆 Partie terminée !', 'success', 5000);

        this.showGameEndScreen(data);
    }

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

    animateNewRound() {
        if (this.elements.roundNumber) {
            animateElement(this.elements.roundNumber, 'animate-bounce');
        }
    }

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


let blindTestGame;

function initBlindTest(config = {}) {
    console.log('🎵 Initialisation Blind Test:', config);
    blindTestGame = new BlindTestGame();
    
    if (config.time_per_round) {
        blindTestGame.timePerRound = config.time_per_round;
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const gameContainer = document.getElementById('game-container');
    const isBlindTestPage = gameContainer && document.querySelector('#answer-input');
    
    if (isBlindTestPage) {
        initBlindTest();
    }
});

window.BlindTestGame = BlindTestGame;
window.initBlindTest = initBlindTest;