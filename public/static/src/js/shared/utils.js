const ESCAPE_ENTITIES = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;"
};

export function escapeHTML(value = "") {
    return value.replace(/[&<>"']/g, (char) => ESCAPE_ENTITIES[char] || char);
}

export function initLavaLampCanvas(selector = "#bg-canvas") {
    const canvas = document.querySelector(selector);
    if (!canvas || !canvas.getContext) {
        return null;
    }

    const ctx = canvas.getContext("2d");
    const state = {
        width: 0,
        height: 0,
        dpr: window.devicePixelRatio || 1
    };

    const blobs = Array.from({ length: 8 }, (_, index) => ({
        x: Math.random(),
        y: Math.random(),
        vx: (Math.random() * 0.4 + 0.1) * (Math.random() > 0.5 ? 1 : -1),
        vy: (Math.random() * 0.4 + 0.1) * (Math.random() > 0.5 ? 1 : -1),
        radius: 160 + Math.random() * 200,
        hue: 215 + index * 8
    }));

    function resize() {
        state.width = window.innerWidth;
        state.height = window.innerHeight;
        state.dpr = window.devicePixelRatio || 1;
        canvas.width = state.width * state.dpr;
        canvas.height = state.height * state.dpr;
        canvas.style.width = `${state.width}px`;
        canvas.style.height = `${state.height}px`;
        ctx.setTransform(state.dpr, 0, 0, state.dpr, 0, 0);
    }

    function render() {
        ctx.clearRect(0, 0, state.width, state.height);

        blobs.forEach((blob) => {
            blob.x += blob.vx * 0.002;
            blob.y += blob.vy * 0.002;

            if (blob.x <= 0 || blob.x >= 1) {
                blob.vx *= -1;
            }
            if (blob.y <= 0 || blob.y >= 1) {
                blob.vy *= -1;
            }

            const centerX = blob.x * state.width;
            const centerY = blob.y * state.height;
            const gradient = ctx.createRadialGradient(
                centerX,
                centerY,
                blob.radius * 0.25,
                centerX,
                centerY,
                blob.radius
            );

            gradient.addColorStop(0, `hsla(${blob.hue}, 85%, 60%, 0.3)`);
            gradient.addColorStop(1, "hsla(220, 70%, 5%, 0)");

            ctx.beginPath();
            ctx.fillStyle = gradient;
            ctx.arc(centerX, centerY, blob.radius, 0, Math.PI * 2);
            ctx.fill();
        });

        requestAnimationFrame(render);
    }

    resize();
    window.addEventListener("resize", resize);
    requestAnimationFrame(render);

    return {
        destroy() {
            window.removeEventListener("resize", resize);
        }
    };
}
