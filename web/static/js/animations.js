function createConfetti() {
    var container = document.getElementById('confetti');
    if (!container) {
        container = document.createElement('div');
        container.id = 'confetti';
        container.className = 'confetti-container';
        document.body.appendChild(container);
    }

    var colors = ['#ff9500', '#ffc107', '#4caf50', '#2196f3', '#e91e63', '#9c27b0'];
    var confettiCount = 150;

    for (var i = 0; i < confettiCount; i++) {
        setTimeout(function() {
            var confetti = document.createElement('div');
            confetti.className = 'confetti';
            confetti.style.left = Math.random() * 100 + '%';
            confetti.style.backgroundColor = colors[Math.floor(Math.random() * colors.length)];
            confetti.style.animationDuration = (Math.random() * 2 + 2) + 's';
            confetti.style.animationDelay = Math.random() * 0.5 + 's';
            
            var size = Math.random() * 10 + 5;
            confetti.style.width = size + 'px';
            confetti.style.height = size + 'px';
            
            if (Math.random() > 0.5) {
                confetti.style.borderRadius = '50%';
            }
            
            container.appendChild(confetti);

            setTimeout(function() {
                confetti.remove();
            }, 4000);
        }, i * 20);
    }
}

function animateScore(element, startValue, endValue, duration) {
    var startTime = null;
    var diff = endValue - startValue;

    function step(timestamp) {
        if (!startTime) startTime = timestamp;
        var progress = Math.min((timestamp - startTime) / duration, 1);
        
        var easeProgress = 1 - Math.pow(1 - progress, 3);
        var currentValue = Math.floor(startValue + diff * easeProgress);
        
        element.textContent = currentValue;

        if (progress < 1) {
            requestAnimationFrame(step);
        } else {
            element.textContent = endValue;
        }
    }

    requestAnimationFrame(step);
}

function pulseElement(element) {
    element.style.transition = 'transform 0.3s ease';
    element.style.transform = 'scale(1.1)';
    
    setTimeout(function() {
        element.style.transform = 'scale(1)';
    }, 300);
}

function shakeElement(element) {
    element.classList.add('shake');
    
    setTimeout(function() {
        element.classList.remove('shake');
    }, 500);
}

function fadeIn(element, duration) {
    duration = duration || 300;
    element.style.opacity = '0';
    element.style.display = 'block';
    element.style.transition = 'opacity ' + duration + 'ms ease';
    
    setTimeout(function() {
        element.style.opacity = '1';
    }, 10);
}

function fadeOut(element, duration, callback) {
    duration = duration || 300;
    element.style.transition = 'opacity ' + duration + 'ms ease';
    element.style.opacity = '0';
    
    setTimeout(function() {
        element.style.display = 'none';
        if (callback) callback();
    }, duration);
}

function slideIn(element, direction) {
    direction = direction || 'left';
    var transforms = {
        left: 'translateX(-100%)',
        right: 'translateX(100%)',
        top: 'translateY(-100%)',
        bottom: 'translateY(100%)'
    };

    element.style.transform = transforms[direction];
    element.style.opacity = '0';
    element.style.display = 'block';
    element.style.transition = 'transform 0.4s ease, opacity 0.4s ease';

    setTimeout(function() {
        element.style.transform = 'translate(0, 0)';
        element.style.opacity = '1';
    }, 10);
}

function highlightCorrect(element) {
    element.style.transition = 'background-color 0.3s ease, box-shadow 0.3s ease';
    element.style.backgroundColor = 'rgba(76, 175, 80, 0.3)';
    element.style.boxShadow = '0 0 20px rgba(76, 175, 80, 0.5)';

    setTimeout(function() {
        element.style.backgroundColor = '';
        element.style.boxShadow = '';
    }, 1500);
}

function highlightIncorrect(element) {
    element.style.transition = 'background-color 0.3s ease, box-shadow 0.3s ease';
    element.style.backgroundColor = 'rgba(244, 67, 54, 0.3)';
    element.style.boxShadow = '0 0 20px rgba(244, 67, 54, 0.5)';

    setTimeout(function() {
        element.style.backgroundColor = '';
        element.style.boxShadow = '';
    }, 1500);
}

function typeWriter(element, text, speed) {
    speed = speed || 50;
    var index = 0;
    element.textContent = '';

    function type() {
        if (index < text.length) {
            element.textContent += text.charAt(index);
            index++;
            setTimeout(type, speed);
        }
    }

    type();
}

function countdownAnimation(element, seconds, callback) {
    var count = seconds;

    function tick() {
        element.textContent = count;
        element.style.transform = 'scale(1.5)';
        element.style.transition = 'transform 0.5s ease';

        setTimeout(function() {
            element.style.transform = 'scale(1)';
        }, 250);

        count--;

        if (count >= 0) {
            setTimeout(tick, 1000);
        } else {
            if (callback) callback();
        }
    }

    tick();
}

function createRipple(event) {
    var button = event.currentTarget;
    var ripple = document.createElement('span');
    var rect = button.getBoundingClientRect();
    var size = Math.max(rect.width, rect.height);
    var x = event.clientX - rect.left - size / 2;
    var y = event.clientY - rect.top - size / 2;

    ripple.style.cssText = 
        'position: absolute; width: ' + size + 'px; height: ' + size + 'px;' +
        'left: ' + x + 'px; top: ' + y + 'px;' +
        'background: rgba(255, 255, 255, 0.3); border-radius: 50%;' +
        'transform: scale(0); animation: ripple 0.6s linear;' +
        'pointer-events: none;';

    button.style.position = 'relative';
    button.style.overflow = 'hidden';
    button.appendChild(ripple);

    setTimeout(function() {
        ripple.remove();
    }, 600);
}

function initRippleButtons() {
    var buttons = document.querySelectorAll('.btn');
    buttons.forEach(function(button) {
        button.addEventListener('click', createRipple);
    });
}

function animateProgressBar(element, targetPercent, duration) {
    duration = duration || 1000;
    var startWidth = parseFloat(element.style.width) || 0;
    var startTime = null;

    function animate(timestamp) {
        if (!startTime) startTime = timestamp;
        var progress = Math.min((timestamp - startTime) / duration, 1);
        var currentWidth = startWidth + (targetPercent - startWidth) * progress;
        element.style.width = currentWidth + '%';

        if (progress < 1) {
            requestAnimationFrame(animate);
        }
    }

    requestAnimationFrame(animate);
}

function showNotification(message, type, duration) {
    type = type || 'info';
    duration = duration || 3000;

    var notification = document.createElement('div');
    notification.className = 'notification notification-' + type;
    notification.textContent = message;
    notification.style.cssText = 
        'position: fixed; top: 20px; right: 20px; padding: 15px 25px;' +
        'border-radius: 10px; color: white; font-weight: bold;' +
        'transform: translateX(120%); transition: transform 0.3s ease;' +
        'z-index: 10000; max-width: 300px;';

    var colors = {
        info: '#2196f3',
        success: '#4caf50',
        error: '#f44336',
        warning: '#ff9800'
    };
    notification.style.backgroundColor = colors[type] || colors.info;

    document.body.appendChild(notification);

    setTimeout(function() {
        notification.style.transform = 'translateX(0)';
    }, 10);

    setTimeout(function() {
        notification.style.transform = 'translateX(120%)';
        setTimeout(function() {
            notification.remove();
        }, 300);
    }, duration);
}

var styleSheet = document.createElement('style');
styleSheet.textContent = 
    '@keyframes ripple { to { transform: scale(4); opacity: 0; } }' +
    '@keyframes shake { 0%, 100% { transform: translateX(0); } 25% { transform: translateX(-10px); } 75% { transform: translateX(10px); } }' +
    '.shake { animation: shake 0.5s ease-in-out; }';
document.head.appendChild(styleSheet);

document.addEventListener('DOMContentLoaded', function() {
    initRippleButtons();
});