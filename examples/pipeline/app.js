class ShowcaseApp {
    constructor() {
        this.logWindow = document.getElementById('log-window');
        this.isAnimating = false;
        
        // Nodes
        this.nodes = {
            app: document.querySelector('#node-app .node'),
            cp: document.querySelector('#node-cp .node'),
            policy: document.querySelector('#node-policy .node'),
            audit: document.querySelector('#node-audit .node'),
            user: document.querySelector('#node-user .node')
        };
        
        // Packets
        this.packets = {
            p1: document.getElementById('packet-1'),
            p2: document.getElementById('packet-2'),
            policy: document.getElementById('packet-policy'),
            audit: document.getElementById('packet-audit')
        };
    }

    log(message, type = 'info') {
        const time = new Date().toISOString().split('T')[1].slice(0, 12);
        const line = document.createElement('div');
        line.className = `log-line ${type}`;
        line.innerHTML = `[${time}] ${message}`;
        this.logWindow.appendChild(line);
        this.logWindow.scrollTop = this.logWindow.scrollHeight;
    }

    clearLogs() {
        this.logWindow.innerHTML = '<div class="log-line system">[SYSTEM] Logs cleared. Awaiting requests...</div>';
    }

    resetPaths() {
        Object.values(this.packets).forEach(p => {
            if (p) {
                p.className = 'packet';
            }
        });
        Object.values(this.nodes).forEach(n => {
            if (n) {
                n.className = n.className.replace(/pulse-\w+/g, '').trim();
            }
        });
    }

    pulseNode(nodeKey, type, duration = 1000) {
        const node = this.nodes[nodeKey];
        node.classList.add(`pulse-${type}`);
        setTimeout(() => {
            node.classList.remove(`pulse-${type}`);
        }, duration);
    }

    async wait(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    async simulate(scenario) {
        if (this.isAnimating) {
            this.log("Please wait for current simulation to finish.", "warn");
            return;
        }
        this.isAnimating = true;
        this.resetPaths();
        this.log(`--- Starting new SDK scenario: ${scenario.toUpperCase()} ---`, "system");

        try {
            // User sends request
            this.log("User sending HTTP request to Application...", "info");
            this.pulseNode('user', 'green', 500);
            
            // Move packet 1 to App
            this.packets.p1.classList.add('anim-move-right');
            await this.wait(1000);
            
            this.log("Application received request.", "info");
            this.pulseNode('app', 'yellow', 1000);

            if (scenario === 'ratelimit') {
                this.log("App SDK: Enforcing local token bucket rate limit...", "info");
                await this.wait(500);
                this.log("SDK Error: 429 Too Many Requests.", "error");
                this.pulseNode('app', 'red', 1000);
                this.isAnimating = false;
                return;
            }

            if (scenario === 'threat') {
                this.log("App SDK: Threat Detection analyzing payload locally...", "info");
                await this.wait(500);
                this.log("SDK Error: Suspicious payload detected (SQLi signature).", "error");
                this.pulseNode('app', 'red', 1000);
                this.isAnimating = false;
                return;
            }

            if (scenario === 'unauth') {
                this.log("App SDK: Validating JWT signature locally...", "info");
                await this.wait(500);
                this.log("SDK Error: 401 Unauthorized (Invalid or missing JWT).", "error");
                this.pulseNode('app', 'red', 1000);
                this.isAnimating = false;
                return;
            }

            this.log("App SDK: JWT Validated. Calling Control Plane POST `/v1/policy/evaluate`", "success");
            this.packets.p2.classList.add('anim-move-right');
            await this.wait(1000);

            this.pulseNode('cp', 'yellow', 1000);
            this.log("Control Plane: Validating Connector API Key.", "info");
            await this.wait(500);
            this.pulseNode('cp', 'green', 1000);
            
            this.log("Control Plane: Delegating authorization to Policy Engine.", "info");
            this.packets.policy.classList.add('anim-move-down');
            await this.wait(1000);

            if (scenario === 'unauthz') {
                this.pulseNode('policy', 'red', 1500);
                this.log("Policy Engine: Checking RBAC...", "info");
                await this.wait(500);
                this.log("Policy Engine Error: Action `write` DENIED for role `Viewer`.", "error");
                this.log("Control Plane: Responding 403 Forbidden to App.", "error");
                this.pulseNode('cp', 'red', 1000);
                this.isAnimating = false;
                return;
            }

            // Success scenario
            this.pulseNode('policy', 'green', 1000);
            this.log("Policy Engine: Result ALLOWED.", "success");
            await this.wait(500);
            
            this.log("Control Plane: Returning 200 OK (Permitted) to Application.", "success");
            this.pulseNode('app', 'green', 1500);
            this.log("Application: Processing business logic successfully.", "success");
            await this.wait(800);

            // Audit
            this.log("App SDK: Emitting EventEnvelope to Control Plane /v1/events/ingest.", "info");
            this.packets.p2.classList.remove('anim-move-right');
            void this.packets.p2.offsetWidth; // force reflow
            this.packets.p2.classList.add('anim-move-right');
            await this.wait(1000);

            this.pulseNode('cp', 'green', 1000);
            this.log("Control Plane: Relaying event to Audit Outbox.", "info");
            
            this.packets.audit.classList.add('anim-move-up');
            await this.wait(1000);

            this.pulseNode('audit', 'green', 1000);
            this.log("Audit Service: Immutable record saved successfully.", "success");
            this.log("Lifecycle Complete. 200 OK returned to User.", "success");

        } catch (err) {
            console.error(err);
        } finally {
            this.isAnimating = false;
        }
    }
}

window.onload = () => {
    window.app = new ShowcaseApp();
};
