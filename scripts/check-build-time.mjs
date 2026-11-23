import { spawnSync } from 'node:child_process'
import path from 'node:path'

// Invoke Vite directly: `npm run build` spends 0.5-0.8s in npm startup before
// Vite even loads. This guard measures the build that actually ships.
const root = process.cwd()
const vite = path.join(root, 'node_modules', 'vite', 'bin', 'vite.js')
const runs = 5
const limitMs = 2000
const timings = []

for (let i = 0; i < runs; i++) {
  const start = performance.now()
  const res = spawnSync(process.execPath, [vite, 'build'], { cwd: root, encoding: 'utf8' })
  const ms = performance.now() - start
  if (res.status !== 0) {
    console.error(res.stdout || '')
    console.error(res.stderr || '')
    process.exit(1)
  }
  const built = (res.stdout.match(/built in ([\d.]+)ms/) || [])[1] || '?'
  timings.push(ms)
  console.log(`run ${i + 1}: ${ms.toFixed(1)}ms wall, ${built}ms vite`)
}

const sorted = [...timings].sort((a, b) => a - b)
console.log(`min=${sorted[0].toFixed(1)}ms median=${sorted[Math.floor(runs / 2)].toFixed(1)}ms max=${sorted[runs - 1].toFixed(1)}ms limit=${limitMs}ms`)
if (sorted[runs - 1] > limitMs) {
  console.error(`FAIL: slowest build exceeded ${limitMs}ms`)
  process.exit(1)
}
