export function animateScoreCells(table, teams = [], scoreHistory, scoreAnimationTokens) {
    if (!table) {
        return;
    }
    const duration = 600;
    teams.forEach(function(team, index) {
        const teamKey = team.id ?? index;
        const attrId = `team-${teamKey}`;
        const cell = table.querySelector(`[data-team-id="${attrId}"]`);
        if (!cell) {
            return;
        }

        const target = Number.isFinite(Number(team.score)) ? Number(team.score) : 0;
        const previous = scoreHistory.get(attrId);
        const startValue = Number.isFinite(previous) ? previous : 0;
        if (startValue === target) {
            cell.textContent = target;
            scoreHistory.set(attrId, target);
            return;
        }

        const token = Symbol();
        scoreAnimationTokens.set(attrId, token);
        const startTime = performance.now();

        function step(timestamp) {
            if (scoreAnimationTokens.get(attrId) !== token) {
                return;
            }
            const progress = Math.min((timestamp - startTime) / duration, 1);
            const current = Math.round(startValue + (target - startValue) * progress);
            cell.textContent = current;
            if (progress < 1) {
                requestAnimationFrame(step);
            } else {
                scoreHistory.set(attrId, target);
            }
        }

        requestAnimationFrame(step);
    });
}
