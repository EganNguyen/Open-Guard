class ShowcaseApp {
    constructor() {
        this.logWindow = document.getElementById('log-window');
        this.isAnimating = false;
        
        // Nodes
        this.nodes = {
            gateway: document.querySelector('#node-gateway .node'),
            policy: document.querySelector('#node-policy .node'),
            main: document.querySelector('#node-main .node'),
            audit: document.querySelector('#node-audit .node'),
            user: document.querySelector('#node-user .node')
        };
        
        // Packets
        this.packets = {
            p1: document.getElementById('packet-1'),
            p2: document.getElementById('packet-2'),
            policy: document.getElementById('packet-policy'),
            auditV: document.getElementById('packet-audit-v'),
            auditH: document.getElementById('packet-audit-h')
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

    async runAnimation(element, animClass, durationMS) {
        element.classList.remove(animClass);
        // Force reflow
        void element.offsetWidth;
        element.classList.add(animClass);
        await this.wait(durationMS);
        element.classList.remove(animClass);
    }

    async simulate(scenario) {
        if (this.isAnimating) {
            this.log("Please wait for current simulation to finish.", "warn");
            return;
        }
        this.isAnimating = true;
        this.resetPaths();
        this.log(`--- Starting new scenario: ${scenario.toUpperCase()} ---`, "system");

        try {
            // STEP 1: User sends request
            this.log("User sending HTTP request to `/api/v1/resource`...", "info");
            this.pulseNode('user', 'green', 500);
            
            // Move packet 1 to Gateway
            this.packets.p1.classList.add('anim-move-right');
            await this.wait(1000);
            
            // Reached Gateway
            this.log("Request intercepted by OpenGuard Gateway.", "info");

            if (scenario === 'ratelimit') {
                this.pulseNode('gateway', 'yellow', 1500);
                this.log("Gateway: Rate limiting check...", "info");
                await this.wait(500);
                this.log("Gateway Error: 429 Too Many Requests (TokenBucket exhausted).", "error");
                this.log("Request blocked gracefully before touching Main Product.", "success");
                this.isAnimating = false;
                return;
            }

            if (scenario === 'threat') {
                this.pulseNode('gateway', 'red', 1500);
                this.log("Gateway: Threat Detection Middleware analyzing payload...", "info");
                await this.wait(500);
                this.log("Gateway Error: Suspicious payload detected (SQLi signature).", "error");
                this.log("Request DROPPED instantly.", "success");
                this.isAnimating = false;
                return;
            }

            if (scenario === 'unauth') {
                this.pulseNode('gateway', 'red', 1500);
                this.log("Gateway: Validating JWT via IAM Service...", "info");
                await this.wait(500);
                this.log("Gateway Error: 401 Unauthorized (Invalid or expired JWT signature).", "error");
                this.log("Request DROPPED instantly.", "success");
                this.isAnimating = false;
                return;
            }

            // Valid Auth path
            this.pulseNode('gateway', 'green', 1000);
            this.log("Gateway: Authentication OK. Extracting `X-User-ID`.", "success");
            await this.wait(500);

            if (scenario === 'unauthz') {
                this.log("Gateway: Delegating authorization to Policy Engine.", "info");
                this.packets.policy.classList.add('anim-move-down');
                await this.wait(1000);
                
                this.pulseNode('policy', 'red', 1500);
                this.log("Policy Engine: Checking Role-Based Access Control (RBAC)...", "info");
                await this.wait(500);
                this.log("Policy Engine Error: Action `write:resource` DENIED for role `Viewer`.", "error");
                this.log("Gateway Error: 403 Forbidden.", "error");
                this.isAnimating = false;
                return;
            }

            // Success scenario
            this.log("Gateway: Delegating authorization to Policy Engine.", "info");
            this.packets.policy.classList.add('anim-move-down');
            await this.wait(1000);
            
            this.pulseNode('policy', 'green', 1000);
            this.log("Policy Engine: Result ALLOWED.", "success");
            await this.wait(500);
            
            this.log("Gateway: Request enriched and forwarded to Main Product.", "info");
            this.packets.p2.classList.add('anim-move-right');
            await this.wait(1000);

            this.pulseNode('main', 'green', 1500);
            this.log("Main Product: Processing logic (blindly trusting Gateway headers).", "success");
            await this.wait(800);

            // Audit
            this.log("Gateway: Emitting EventEnvelope to Kafka via Outbox.", "info");
            // Audit path: up then right
            this.packets.auditV.classList.add('anim-move-up');
            await this.wait(1000);
            this.packets.auditH.classList.add('anim-move-right-green');
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

// Initialize global app instance
window.onload = () => {
    window.app = new ShowcaseApp();
};
