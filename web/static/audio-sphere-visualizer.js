/**
 * ============================================================================
 * AUDIO SPHERE VISUALIZER - Module Autonome
 * ============================================================================
 * Visualiseur audio 3D avec sphère réactive aux fréquences
 * 
 * Dépendances: Three.js r128+
 * 
 * Usage:
 *   const vis = new AudioSphereVisualizer('container-id');
 *   vis.connectAudio(audioElement);
 *   vis.start();
 * 
 * @version 1.0.0
 * ============================================================================
 */

(function(global) {
    'use strict';

    // ========================================================================
    // CONFIGURATION PAR DÉFAUT
    // ========================================================================
    const DEFAULT_CONFIG = {
        // Sphère
        sphere: {
            radius: 2,
            segments: 64,
            wireframe: true,
            pointsMode: false,
            pointSize: 2,
        },
        
        // Couleurs
        colors: {
            primary: 0x00d4ff,
            secondary: 0xff00ff,
            tertiary: 0x00ff88,
            background: 0x0a0a0f,
            glow: 0x00d4ff,
        },
        
        // Audio
        audio: {
            fftSize: 256,
            smoothing: 0.8,
            bassMultiplier: 1.5,
            midMultiplier: 1.2,
            highMultiplier: 1.0,
            bassRange: [0, 4],
            midRange: [4, 12],
            highRange: [12, 32],
        },
        
        // Animation
        animation: {
            rotationSpeed: 0.001,
            pulseIntensity: 0.5,
            deformIntensity: 0.3,
            noiseScale: 3,
            noiseSpeed: 0.5,
        },
        
        // Effets
        effects: {
            glowEnabled: true,
            glowIntensity: 1.5,
            particlesEnabled: true,
            particleCount: 200,
        },
        
        // Performance
        performance: {
            adaptiveQuality: true,
            lowQualityThreshold: 30,
        }
    };

    // ========================================================================
    // AUDIO ANALYZER
    // ========================================================================
    class AudioAnalyzer {
        constructor(config) {
            this.config = config;
            this.audioContext = null;
            this.analyser = null;
            this.dataArray = null;
            this.source = null;
            this.isInitialized = false;
        }
        
        async connect(audioElement) {
            if (this.isInitialized) {
                this.disconnect();
            }
            
            try {
                this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
                this.analyser = this.audioContext.createAnalyser();
                this.analyser.fftSize = this.config.fftSize;
                this.analyser.smoothingTimeConstant = this.config.smoothing;
                this.dataArray = new Uint8Array(this.analyser.frequencyBinCount);
                
                this.source = this.audioContext.createMediaElementSource(audioElement);
                this.source.connect(this.analyser);
                this.analyser.connect(this.audioContext.destination);
                
                this.isInitialized = true;
                return true;
            } catch (e) {
                console.error('[AudioAnalyzer] Error:', e);
                return false;
            }
        }
        
        disconnect() {
            if (this.source) {
                try { this.source.disconnect(); } catch(e) {}
                this.source = null;
            }
            this.isInitialized = false;
        }
        
        analyze() {
            if (!this.isInitialized) {
                return { bass: 0, mid: 0, high: 0, average: 0 };
            }
            
            this.analyser.getByteFrequencyData(this.dataArray);
            
            const bass = this._getAverage(this.config.bassRange) * this.config.bassMultiplier;
            const mid = this._getAverage(this.config.midRange) * this.config.midMultiplier;
            const high = this._getAverage(this.config.highRange) * this.config.highMultiplier;
            const average = (bass + mid + high) / 3;
            
            return {
                bass: Math.min(bass / 255, 1),
                mid: Math.min(mid / 255, 1),
                high: Math.min(high / 255, 1),
                average: Math.min(average / 255, 1)
            };
        }
        
        _getAverage(range) {
            let sum = 0;
            for (let i = range[0]; i < range[1] && i < this.dataArray.length; i++) {
                sum += this.dataArray[i];
            }
            return sum / (range[1] - range[0]);
        }
        
        async resume() {
            if (this.audioContext?.state === 'suspended') {
                await this.audioContext.resume();
            }
        }
    }

    // ========================================================================
    // SPHERE VISUALIZER - Classe principale
    // ========================================================================
    class AudioSphereVisualizer {
        constructor(containerId, customConfig = {}) {
            // Merge config
            this.config = this._mergeDeep(DEFAULT_CONFIG, customConfig);
            
            // Container
            this.container = typeof containerId === 'string' 
                ? document.getElementById(containerId) 
                : containerId;
            
            if (!this.container) {
                throw new Error(`Container "${containerId}" not found`);
            }
            
            // Three.js
            this.scene = null;
            this.camera = null;
            this.renderer = null;
            this.sphere = null;
            this.sphereGeometry = null;
            this.originalPositions = null;
            this.particles = null;
            this.glowSphere = null;
            
            // State
            this.clock = new THREE.Clock();
            this.isRunning = false;
            this.animationId = null;
            this.audioData = { bass: 0, mid: 0, high: 0, average: 0 };
            
            // Audio
            this.analyzer = new AudioAnalyzer(this.config.audio);
            this.audioElement = null;
            this.isAudioConnected = false;
            
            // Init
            this._init();
        }
        
        // ====================================================================
        // INITIALISATION
        // ====================================================================
        _init() {
            // Scene
            this.scene = new THREE.Scene();
            this.scene.background = new THREE.Color(this.config.colors.background);
            
            // Camera
            const aspect = this.container.clientWidth / this.container.clientHeight;
            this.camera = new THREE.PerspectiveCamera(75, aspect, 0.1, 1000);
            this.camera.position.z = 5;
            
            // Renderer
            this.renderer = new THREE.WebGLRenderer({ 
                antialias: true, 
                alpha: true,
                powerPreference: 'high-performance'
            });
            this.renderer.setSize(this.container.clientWidth, this.container.clientHeight);
            this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
            this.container.appendChild(this.renderer.domElement);
            
            // Objects
            this._createSphere();
            if (this.config.effects.glowEnabled) this._createGlow();
            if (this.config.effects.particlesEnabled) this._createParticles();
            this._createLights();
            
            // Events
            this._boundResize = this._onResize.bind(this);
            window.addEventListener('resize', this._boundResize);
        }
        
        _createSphere() {
            const { radius, segments, wireframe, pointsMode, pointSize } = this.config.sphere;
            
            this.sphereGeometry = new THREE.IcosahedronGeometry(radius, Math.floor(segments / 16));
            this.originalPositions = this.sphereGeometry.attributes.position.array.slice();
            
            let material;
            if (pointsMode) {
                material = new THREE.PointsMaterial({
                    color: this.config.colors.primary,
                    size: pointSize * 0.01,
                    transparent: true,
                    opacity: 0.8,
                    blending: THREE.AdditiveBlending
                });
                this.sphere = new THREE.Points(this.sphereGeometry, material);
            } else {
                material = new THREE.MeshBasicMaterial({
                    color: this.config.colors.primary,
                    wireframe: wireframe,
                    transparent: true,
                    opacity: 0.9
                });
                this.sphere = new THREE.Mesh(this.sphereGeometry, material);
            }
            
            this.scene.add(this.sphere);
        }
        
        _createGlow() {
            const geo = new THREE.IcosahedronGeometry(this.config.sphere.radius * 1.2, 2);
            const mat = new THREE.MeshBasicMaterial({
                color: this.config.colors.glow,
                transparent: true,
                opacity: 0.1,
                side: THREE.BackSide,
                blending: THREE.AdditiveBlending
            });
            this.glowSphere = new THREE.Mesh(geo, mat);
            this.scene.add(this.glowSphere);
        }
        
        _createParticles() {
            const count = this.config.effects.particleCount;
            const geo = new THREE.BufferGeometry();
            const pos = new Float32Array(count * 3);
            const col = new Float32Array(count * 3);
            
            const c1 = new THREE.Color(this.config.colors.primary);
            const c2 = new THREE.Color(this.config.colors.tertiary);
            
            for (let i = 0; i < count; i++) {
                const theta = Math.random() * Math.PI * 2;
                const phi = Math.acos(2 * Math.random() - 1);
                const r = 3 + Math.random() * 4;
                
                pos[i * 3] = r * Math.sin(phi) * Math.cos(theta);
                pos[i * 3 + 1] = r * Math.sin(phi) * Math.sin(theta);
                pos[i * 3 + 2] = r * Math.cos(phi);
                
                const color = c1.clone().lerp(c2, Math.random());
                col[i * 3] = color.r;
                col[i * 3 + 1] = color.g;
                col[i * 3 + 2] = color.b;
            }
            
            geo.setAttribute('position', new THREE.BufferAttribute(pos, 3));
            geo.setAttribute('color', new THREE.BufferAttribute(col, 3));
            
            const mat = new THREE.PointsMaterial({
                size: 0.02,
                vertexColors: true,
                transparent: true,
                opacity: 0.6,
                blending: THREE.AdditiveBlending
            });
            
            this.particles = new THREE.Points(geo, mat);
            this.scene.add(this.particles);
        }
        
        _createLights() {
            this.scene.add(new THREE.AmbientLight(0xffffff, 0.3));
            const point = new THREE.PointLight(this.config.colors.primary, 1, 10);
            point.position.set(2, 2, 2);
            this.scene.add(point);
        }
        
        // ====================================================================
        // PUBLIC API
        // ====================================================================
        
        /**
         * Connecte le visualiseur à un élément audio
         * @param {HTMLAudioElement} audioElement
         */
        async connectAudio(audioElement) {
            this.audioElement = audioElement;
            const success = await this.analyzer.connect(audioElement);
            this.isAudioConnected = success;
            return success;
        }
        
        /**
         * Démarre le visualiseur
         */
        async start() {
            if (this.isAudioConnected) {
                await this.analyzer.resume();
            }
            if (!this.isRunning) {
                this.isRunning = true;
                this._animate();
            }
        }
        
        /**
         * Met en pause
         */
        pause() {
            this.isRunning = false;
            if (this.animationId) {
                cancelAnimationFrame(this.animationId);
            }
        }
        
        /**
         * Arrête et déconnecte
         */
        stop() {
            this.pause();
            this.analyzer.disconnect();
            this.isAudioConnected = false;
        }
        
        /**
         * Change les couleurs en temps réel
         * @param {Object} colors - {primary, secondary, tertiary, glow}
         */
        setColors(colors) {
            if (colors.primary) {
                this.config.colors.primary = colors.primary;
                this.sphere.material.color.setHex(colors.primary);
            }
            if (colors.glow && this.glowSphere) {
                this.glowSphere.material.color.setHex(colors.glow);
            }
        }
        
        /**
         * Libère les ressources
         */
        dispose() {
            this.stop();
            window.removeEventListener('resize', this._boundResize);
            
            this.sphereGeometry?.dispose();
            this.sphere?.material?.dispose();
            this.glowSphere?.geometry?.dispose();
            this.glowSphere?.material?.dispose();
            this.particles?.geometry?.dispose();
            this.particles?.material?.dispose();
            this.renderer?.dispose();
            
            if (this.renderer?.domElement) {
                this.container.removeChild(this.renderer.domElement);
            }
        }
        
        // ====================================================================
        // ANIMATION
        // ====================================================================
        _animate() {
            if (!this.isRunning) return;
            
            this.animationId = requestAnimationFrame(() => this._animate());
            
            // Analyse audio
            if (this.isAudioConnected) {
                const data = this.analyzer.analyze();
                this._smoothAudioData(data);
            }
            
            const time = this.clock.getElapsedTime();
            const { bass, mid, high, average } = this.audioData;
            const anim = this.config.animation;
            
            // Rotation
            this.sphere.rotation.x += anim.rotationSpeed + bass * 0.01;
            this.sphere.rotation.y += anim.rotationSpeed * 1.5 + mid * 0.01;
            
            // Déformation
            const positions = this.sphereGeometry.attributes.position.array;
            for (let i = 0; i < positions.length; i += 3) {
                const ox = this.originalPositions[i];
                const oy = this.originalPositions[i + 1];
                const oz = this.originalPositions[i + 2];
                
                const len = Math.sqrt(ox*ox + oy*oy + oz*oz);
                const nx = ox/len, ny = oy/len, nz = oz/len;
                
                const noise = Math.sin(nx * anim.noiseScale + time * anim.noiseSpeed) *
                             Math.cos(ny * anim.noiseScale + time * anim.noiseSpeed) *
                             Math.sin(nz * anim.noiseScale + time * anim.noiseSpeed * 0.5);
                
                const pulse = 1 + bass * anim.pulseIntensity;
                const deform = noise * anim.deformIntensity * (mid + high * 0.5);
                const scale = pulse + deform;
                
                positions[i] = ox * scale;
                positions[i + 1] = oy * scale;
                positions[i + 2] = oz * scale;
            }
            this.sphereGeometry.attributes.position.needsUpdate = true;
            
            // Couleur dynamique
            const hue = (time * 0.1 + bass * 0.5) % 1;
            this.sphere.material.color.setHSL(hue, 0.8 + high * 0.2, 0.5 + average * 0.3);
            
            // Glow
            if (this.glowSphere) {
                this.glowSphere.rotation.x = -this.sphere.rotation.x * 0.5;
                this.glowSphere.rotation.y = -this.sphere.rotation.y * 0.5;
                this.glowSphere.scale.setScalar(1 + bass * 0.3);
                this.glowSphere.material.opacity = 0.05 + average * 0.15;
            }
            
            // Particules
            if (this.particles) {
                this.particles.rotation.y += 0.0005 + average * 0.002;
            }
            
            this.renderer.render(this.scene, this.camera);
        }
        
        _smoothAudioData(data) {
            const s = 0.3;
            this.audioData.bass = this.audioData.bass * (1-s) + data.bass * s;
            this.audioData.mid = this.audioData.mid * (1-s) + data.mid * s;
            this.audioData.high = this.audioData.high * (1-s) + data.high * s;
            this.audioData.average = this.audioData.average * (1-s) + data.average * s;
        }
        
        _onResize() {
            const w = this.container.clientWidth;
            const h = this.container.clientHeight;
            this.camera.aspect = w / h;
            this.camera.updateProjectionMatrix();
            this.renderer.setSize(w, h);
        }
        
        // ====================================================================
        // UTILS
        // ====================================================================
        _mergeDeep(target, source) {
            const output = { ...target };
            for (const key in source) {
                if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
                    output[key] = this._mergeDeep(target[key] || {}, source[key]);
                } else {
                    output[key] = source[key];
                }
            }
            return output;
        }
    }

    // Export
    global.AudioSphereVisualizer = AudioSphereVisualizer;

})(typeof window !== 'undefined' ? window : this);