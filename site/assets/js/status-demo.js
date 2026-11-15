const bars = document.querySelectorAll('.status-bar')
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
let interval = null

function updateBars() {
  for (const bar of bars) {
    const value = Math.random()
    let status = 'up'
    if (value > 0.97) status = 'down'
    else if (value > 0.94) status = 'degraded'
    bar.dataset.status = status
  }
}

function updateAnimation() {
  clearInterval(interval)
  interval = null

  if (bars.length && !reducedMotion.matches && !document.hidden) {
    interval = setInterval(updateBars, 4000)
  }
}

reducedMotion.addEventListener('change', updateAnimation)
document.addEventListener('visibilitychange', updateAnimation)
window.addEventListener('pageshow', updateAnimation)
window.addEventListener('pagehide', () => clearInterval(interval))

updateAnimation()
