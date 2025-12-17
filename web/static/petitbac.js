class PetitBacGame {
    constructor() {
        this.currentRound = 0;
        this.totalRounds = 9;
        this.currentLetter = '?';
        this.categories = [];
        this.answers = {};
        this.hasSubmitted = false;
        this.isPlaying = false;
        this.isVoting = false;
        this.votes = {};
        this.scores = {};
        this.players = {};

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
            currentLetter: document.getElementById('current-letter'),
            categoriesForm: document.getElementById('categories-form'),
            submitBtn: document.getElementById('submit-answers'),
            stopBtn: document.getElementById('stop-round'),
            votingSection: document.getElementById('voting-section'),
            playersScores: document.getElementById('players-scores'),
            gameContainer: document.getElementById('game-container'),
            actions: document.getElementById('actions')
        };
    }

    setupEventListeners() {
        if (this.elements.submitBtn) {
            this.elements.submitBtn.addEventListener('click', () => this.submitAnswers());
        }

        if (this.elements.stopBtn) {
            this.elements.stopBtn.addEventListener('click', () => this.stopRound());
        }

        if (this.elements.categoriesForm) {
            this.elements.categoriesForm.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    this.focusNextInput(e.target);
                }
            });
        }
    }

    setupWebSocketHandlers() {
        wsManager.on('pb_new_round', (data) => {
            this.handleNewRound(data);
        });

        wsManager.on('pb_answer', (data) => {
            this.handlePlayerAnswer(data);
        });

        wsManager.on('pb_vote', (data) => {
            this.handleVote(data);
        });

        wsManager.on('pb_vote_result', (data) => {
            this.handleVoteResults(data);
        });

        wsManager.on('pb_stop_round', (data) => {
            this.handleStopRound(data);
        });

        wsManager.on('pb_game_end', (data) => {
            this.handleGameEnd(data);
        });

        wsManager.on('pb_scores', (data) => {
            this.updateScores(data);
        });
    }

    handleNewRound(data) {
        console.log('🎼 Nouvelle manche:', data);

        this.currentRound = data.round;
        this.totalRounds = data.total_rounds;
        this.currentLetter = data.letter;
        this.categories = data.categories || [];
        this.answers = {};
        this.hasSubmitted = false;
        this.isPlaying = true;
        this.isVoting = false;
        this.votes = {};

        this.updateRoundInfo();
        this.renderCategoriesForm();
        this.hideVotingSection();
        this.enableForm();

        this.animateLetter();

        showToast(`Manche ${this.currentRound}/${this.totalRounds} - Lettre ${this.currentLetter}`, 'info');
    }

    updateRoundInfo() {
        if (this.elements.roundNumber) {
            this.elements.roundNumber.textContent = `Manche ${this.currentRound}/${this.totalRounds}`;
        }
        if (this.elements.currentLetter) {
            this.elements.currentLetter.textContent = this.currentLetter;
        }
    }

    renderCategoriesForm() {
        if (!this.elements.categoriesForm) return;

        let html = '';

        this.categories.forEach((category, index) => {
            html += `
                <div class="category-input" data-category="${category}">
                    <label for="cat-${category}">${this.formatCategoryName(category)}</label>
                    <input 
                        type="text" 
                        id="cat-${category}" 
                        name="${category}"
                        placeholder="Commencez par ${this.currentLetter}..."
                        autocomplete="off"
                        data-index="${index}"
                    />
                    <span class="validation-icon"></span>
                </div>
            `;
        });

        this.elements.categoriesForm.innerHTML = html;

        this.setupInputValidation();

        const firstInput = this.elements.categoriesForm.querySelector('input');
        if (firstInput) {
            setTimeout(() => firstInput.focus(), 100);
        }
    }

    setupInputValidation() {
        const inputs = this.elements.categoriesForm?.querySelectorAll('input');
        
        inputs?.forEach(input => {
            input.addEventListener('input', (e) => {
                this.validateInput(e.target);
            });
        });
    }

    validateInput(input) {
        const value = input.value.trim().toUpperCase();
        const container = input.closest('.category-input');
        const icon = container?.querySelector('.validation-icon');

        if (!value) {
            container?.classList.remove('valid', 'invalid');
            if (icon) icon.textContent = '';
            return;
        }

        if (value.startsWith(this.currentLetter)) {
            container?.classList.add('valid');
            container?.classList.remove('invalid');
            if (icon) icon.textContent = '✓';
        } else {
            container?.classList.add('invalid');
            container?.classList.remove('valid');
            if (icon) icon.textContent = '✕';
        }
    }

    focusNextInput(currentInput) {
        const inputs = Array.from(this.elements.categoriesForm?.querySelectorAll('input') || []);
        const currentIndex = inputs.indexOf(currentInput);
        
        if (currentIndex < inputs.length - 1) {
            inputs[currentIndex + 1].focus();
        } else {
            if (!this.hasSubmitted) {
                this.submitAnswers();
            }
        }
    }

    submitAnswers() {
        if (this.hasSubmitted || !this.isPlaying) {
            return;
        }

        const inputs = this.elements.categoriesForm?.querySelectorAll('input');
        const answers = {};

        inputs?.forEach(input => {
            const category = input.name;
            const value = input.value.trim();
            answers[category] = value;
        });

        this.answers = answers;
        this.hasSubmitted = true;

        wsManager.submitPetitBacAnswers(answers);

        this.disableForm();

        if (this.elements.stopBtn) {
            this.elements.stopBtn.disabled = false;
        }

        showToast('Réponses soumises !', 'success');
    }

    stopRound() {
        if (!this.hasSubmitted) {
            showToast('Soumettez d\'abord vos réponses', 'warning');
            return;
        }

        wsManager.stopPetitBacRound();
        
        if (this.elements.stopBtn) {
            this.elements.stopBtn.disabled = true;
        }

        showToast('STOP ! ⏱️', 'warning');
    }

    enableForm() {
        const inputs = this.elements.categoriesForm?.querySelectorAll('input');
        inputs?.forEach(input => input.disabled = false);
        
        if (this.elements.submitBtn) {
            this.elements.submitBtn.disabled = false;
            this.elements.submitBtn.style.display = '';
        }
        if (this.elements.stopBtn) {
            this.elements.stopBtn.disabled = true;
            this.elements.stopBtn.style.display = '';
        }
        if (this.elements.actions) {
            this.elements.actions.style.display = '';
        }
    }

    disableForm() {
        const inputs = this.elements.categoriesForm?.querySelectorAll('input');
        inputs?.forEach(input => input.disabled = true);
        
        if (this.elements.submitBtn) {
            this.elements.submitBtn.disabled = true;
        }
    }

    handlePlayerAnswer(data) {
        showToast(`${data.pseudo} a soumis ses réponses`, 'info');
    }

    handleStopRound(data) {
        showToast(`${data.pseudo} a dit STOP !`, 'warning', 3000);
        
        document.body.classList.add('flash-warning');
        setTimeout(() => {
            document.body.classList.remove('flash-warning');
        }, 500);
    }

    handleVote(data) {
        console.log('🗳️ Vote:', data);

        if (data.phase === 'start') {
            this.isPlaying = false;
            this.isVoting = true;
            this.renderVotingSection(data.answers);
            showToast('Phase de vote !', 'info');
        } else if (data.phase === 'vote') {
            this.updateVoteStatus(data);
        }
    }

    renderVotingSection(answersToVote) {
        if (!this.elements.votingSection) return;

        let html = '<h3>🗳️ Votez pour valider les réponses</h3>';

        for (const category in answersToVote) {
            const categoryAnswers = answersToVote[category];
            
            if (categoryAnswers.length === 0) continue;

            html += `
                <div class="vote-category">
                    <h4>${this.formatCategoryName(category)}</h4>
            `;

            categoryAnswers.forEach(answer => {
                const isSelf = answer.user_id === wsManager.userId;
                
                html += `
                    <div class="vote-item" data-user-id="${answer.user_id}" data-category="${category}">
                        <div class="answer-info">
                            <span class="answer-text">${answer.answer}</span>
                            <span class="player-name">par ${answer.pseudo}</span>
                        </div>
                        ${!isSelf ? `
                            <div class="vote-buttons">
                                <button class="vote-btn valid" onclick="petitBacGame.vote(${answer.user_id}, '${category}', true)">
                                    ✓ Valide
                                </button>
                                <button class="vote-btn invalid" onclick="petitBacGame.vote(${answer.user_id}, '${category}', false)">
                                    ✕ Invalide
                                </button>
                            </div>
                        ` : '<span class="text-muted">Votre réponse</span>'}
                    </div>
                `;
            });

            html += '</div>';
        }

        this.elements.votingSection.innerHTML = html;
        this.elements.votingSection.style.display = 'block';
        
        if (this.elements.categoriesForm) {
            this.elements.categoriesForm.style.display = 'none';
        }
        if (this.elements.actions) {
            this.elements.actions.style.display = 'none';
        }

        animateElement(this.elements.votingSection, 'animate-slide-up');
    }

    hideVotingSection() {
        if (this.elements.votingSection) {
            this.elements.votingSection.style.display = 'none';
        }
        if (this.elements.categoriesForm) {
            this.elements.categoriesForm.style.display = '';
        }
    }

    vote(targetUserId, category, isValid) {
        const key = `${category}-${targetUserId}`;
        
        if (this.votes[key] !== undefined) {
            showToast('Vous avez déjà voté pour cette réponse', 'warning');
            return;
        }

        this.votes[key] = isValid;

        wsManager.submitPetitBacVote(targetUserId, category, isValid);

        const voteItem = document.querySelector(
            `.vote-item[data-user-id="${targetUserId}"][data-category="${category}"]`
        );
        
        if (voteItem) {
            const buttons = voteItem.querySelectorAll('.vote-btn');
            buttons.forEach(btn => {
                btn.disabled = true;
                btn.classList.remove('selected');
            });

            const selectedBtn = voteItem.querySelector(`.vote-btn.${isValid ? 'valid' : 'invalid'}`);
            if (selectedBtn) {
                selectedBtn.classList.add('selected');
            }
        }

        showToast(`Vote enregistré : ${isValid ? '✓ Valide' : '✕ Invalide'}`, 'success');
    }

    updateVoteStatus(data) {
        console.log(`${data.voter_id} a voté pour ${data.target_id} (${data.category})`);
    }

    handleVoteResults(data) {
        console.log('📊 Résultats votes:', data);

        this.isVoting = false;

        this.showVoteResults(data.results);

        if (data.scores) {
            this.updateScores(data.scores);
        }
    }

    showVoteResults(results) {
        if (!this.elements.votingSection) return;

        let html = '<h3>📊 Résultats de la manche</h3>';

        for (const category in results) {
            const categoryResults = results[category];
            
            html += `
                <div class="vote-category">
                    <h4>${this.formatCategoryName(category)}</h4>
            `;

            categoryResults.forEach(result => {
                const statusClass = result.is_valid ? 'correct' : 'wrong';
                html += `
                    <div class="result-item ${statusClass}">
                        <div class="result-info">
                            <span class="player-name">${result.pseudo}</span>
                            <span class="answer-text">${result.answer || '-'}</span>
                        </div>
                        <div class="result-details">
                            <span class="votes-info">
                                ✓${result.votes_for} / ✕${result.votes_against}
                            </span>
                            <span class="result-points ${result.points > 0 ? 'text-success' : 'text-danger'}">
                                +${result.points} pt${result.points > 1 ? 's' : ''}
                            </span>
                        </div>
                    </div>
                `;
            });

            html += '</div>';
        }

        this.elements.votingSection.innerHTML = html;
        animateElement(this.elements.votingSection, 'animate-fade-in');
    }

    updateScores(scores) {
        this.scores = scores;
        this.renderScoreboard();
    }

    renderScoreboard() {
        if (!this.elements.playersScores) return;

        const entries = Object.entries(this.scores).map(([id, score]) => ({
            userId: parseInt(id),
            score: score
        }));
        
        entries.sort((a, b) => b.score - a.score);

        let html = '<h3>🏆 Scores</h3><div class="scoreboard">';
        
        entries.forEach((entry, index) => {
            const rankClass = index < 3 ? `rank-${index + 1}` : '';
            const rankEmoji = index === 0 ? '🥇' : index === 1 ? '🥈' : index === 2 ? '🥉' : `${index + 1}.`;
            
            html += `
                <div class="score-row ${rankClass}">
                    <span class="score-rank">${rankEmoji}</span>
                    <span class="score-player">Joueur #${entry.userId}</span>
                    <span class="score-value">${entry.score} pts</span>
                </div>
            `;
        });

        html += '</div>';
        this.elements.playersScores.innerHTML = html;
    }

    handleGameEnd(data) {
        console.log('🏆 Fin de partie:', data);

        this.isPlaying = false;
        this.isVoting = false;

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

    formatCategoryName(category) {
        const icons = {
            'artiste': '🎤 Artiste',
            'album': '💿 Album',
            'groupe': '🎸 Groupe',
            'instrument': '🎹 Instrument',
            'featuring': '🤝 Featuring'
        };
        
        return icons[category.toLowerCase()] || 
               category.charAt(0).toUpperCase() + category.slice(1);
    }

    animateLetter() {
        if (this.elements.currentLetter) {
            this.elements.currentLetter.style.animation = 'none';
            setTimeout(() => {
                this.elements.currentLetter.style.animation = '';
                animateElement(this.elements.currentLetter, 'animate-bounce');
            }, 10);
        }
    }
}


let petitBacGame;

function initPetitBac(config = {}) {
    console.log('🎼 Initialisation Petit Bac:', config);
    petitBacGame = new PetitBacGame();
    
    if (config.categories) {
        petitBacGame.categories = config.categories;
    }
    if (config.nb_rounds) {
        petitBacGame.totalRounds = config.nb_rounds;
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const gameContainer = document.getElementById('game-container');
    const isPetitBacPage = gameContainer && document.querySelector('#current-letter');
    
    if (isPetitBacPage) {
        initPetitBac();
    }
});

window.PetitBacGame = PetitBacGame;
window.initPetitBac = initPetitBac;
window.petitBacGame = null; // Sera initialisé