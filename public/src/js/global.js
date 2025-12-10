import { initLavaLampCanvas } from "./shared/utils.js";

const COUNTER_DURATION = 900;

document.addEventListener("DOMContentLoaded", () => {
    initLavaLampCanvas();
    animateCounters();
});

function animateCounters() {
    const counters = document.querySelectorAll("[data-counter]");
    counters.forEach((element) => {
        const rawTarget = element.dataset.target ?? element.textContent?.trim();
        const target = Number(rawTarget);
        if (!Number.isFinite(target)) {
            return;
        }
        const startValue = Number(element.dataset.start) || 0;
        let startTime = null;
        element.textContent = startValue;

        function step(timestamp) {
            if (!startTime) {
                startTime = timestamp;
            }
            const progress = Math.min((timestamp - startTime) / COUNTER_DURATION, 1);
            const current = Math.round(startValue + (target - startValue) * progress);
            element.textContent = current;
            if (progress < 1) {
                requestAnimationFrame(step);
            } else {
                element.textContent = target;
            }
        }

        requestAnimationFrame(step);
    });
}
