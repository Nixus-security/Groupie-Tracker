var ws = null;
var wsReconnectAttempts = 0;
var wsMaxReconnectAttempts = 5;
var wsReconnectDelay = 2000;

function initWebSocket(roomCode) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        return;
    }

    var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var wsUrl = protocol + '//' + window.location.host + '/ws/' + roomCode;

    try {
        ws = new WebSocket(wsUrl);
    } catch (e) {
        console.error('WebSocket creation failed:', e);
        return;
    }

    ws.onopen = function() {
        console.log('WebSocket connected');
        wsReconnectAttempts = 0;
        
        if (typeof onWebSocketOpen === 'function') {
            onWebSocketOpen();
        }
    };

    ws.onmessage = function(event) {
        try {
            var data = JSON.parse(event.data);
            
            if (typeof onWebSocketMessage === 'function') {
                onWebSocketMessage(data);
            }
        } catch (e) {
            console.error('Failed to parse WebSocket message:', e);
        }
    };

    ws.onclose = function(event) {
        console.log('WebSocket closed:', event.code, event.reason);
        
        if (typeof onWebSocketClose === 'function') {
            onWebSocketClose(event);
        }

        if (wsReconnectAttempts < wsMaxReconnectAttempts) {
            wsReconnectAttempts++;
            console.log('Reconnecting... Attempt ' + wsReconnectAttempts);
            
            setTimeout(function() {
                initWebSocket(roomCode);
            }, wsReconnectDelay * wsReconnectAttempts);
        } else {
            console.error('Max reconnection attempts reached');
            showConnectionError();
        }
    };

    ws.onerror = function(error) {
        console.error('WebSocket error:', error);
        
        if (typeof onWebSocketError === 'function') {
            onWebSocketError(error);
        }
    };
}

function sendMessage(message) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(message));
        return true;
    } else {
        console.warn('WebSocket not connected');
        return false;
    }
}

function closeWebSocket() {
    if (ws) {
        ws.close();
        ws = null;
    }
}

function isWebSocketConnected() {
    return ws && ws.readyState === WebSocket.OPEN;
}

function showConnectionError() {
    var overlay = document.createElement('div');
    overlay.className = 'connection-error-overlay';
    overlay.innerHTML = 
        '<div class="connection-error-content">' +
            '<span class="error-icon">⚠️</span>' +
            '<h2>Connexion perdue</h2>' +
            '<p>Impossible de se reconnecter au serveur.</p>' +
            '<button class="btn btn-primary" onclick="location.reload()">Rafraîchir la page</button>' +
        '</div>';
    
    overlay.style.cssText = 
        'position: fixed; top: 0; left: 0; width: 100%; height: 100%;' +
        'background: rgba(26, 26, 46, 0.95); display: flex;' +
        'align-items: center; justify-content: center; z-index: 9999;';
    
    var content = overlay.querySelector('.connection-error-content');
    content.style.cssText = 'text-align: center; color: white;';
    
    var icon = overlay.querySelector('.error-icon');
    icon.style.cssText = 'font-size: 4rem; display: block; margin-bottom: 20px;';
    
    document.body.appendChild(overlay);
}

function sendChat(message) {
    return sendMessage({
        type: 'chat',
        data: { message: message }
    });
}

function sendReady() {
    return sendMessage({
        type: 'ready'
    });
}

function sendStartGame() {
    return sendMessage({
        type: 'start_game'
    });
}

function sendAnswer(answer) {
    return sendMessage({
        type: 'submit_answer',
        data: { answer: answer }
    });
}

function sendPing() {
    return sendMessage({
        type: 'ping'
    });
}

setInterval(function() {
    if (isWebSocketConnected()) {
        sendPing();
    }
}, 30000);

window.addEventListener('beforeunload', function() {
    closeWebSocket();
});