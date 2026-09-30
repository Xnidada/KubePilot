const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')

// Exercise the hook's asynchronous request guards without adding a DOM/test
// framework. This tiny hook runner preserves state, refs and effect dependencies.
function harness() {
  const slots = []
  const effects = []
  const requests = []
  let cursor = 0
  const same = (a, b) => a && b && a.length === b.length && a.every((v, i) => Object.is(v, b[i]))
  const react = {
    useState(initial) {
      const i = cursor++
      if (!(i in slots)) slots[i] = initial
      return [slots[i], value => { slots[i] = typeof value === 'function' ? value(slots[i]) : value }]
    },
    useRef(initial) {
      const i = cursor++
      if (!(i in slots)) slots[i] = { current: initial }
      return slots[i]
    },
    useCallback(callback, deps) {
      const i = cursor++
      if (!same(slots[i]?.deps, deps)) slots[i] = { deps, callback }
      return slots[i].callback
    },
    useEffect(effect, deps) {
      const i = cursor++
      if (!same(slots[i]?.deps, deps)) {
        effects.push(() => {
          slots[i]?.cleanup?.()
          slots[i] = { deps, cleanup: effect() }
        })
      }
    },
  }
  const api = {
    getConversations: async () => ({ code: 0, data: [] }),
    getConversation: id => new Promise(resolve => requests.push({ id, resolve })),
  }
  const source = fs.readFileSync(path.join(__dirname, '../src/hooks/useConversations.ts'), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
  const exports = {}
  vm.runInNewContext(compiled, {
    exports, console,
    require(name) {
      if (name === 'react') return react
      if (name === 'antd') return { message: {} }
      if (name === '../api/conversation') return api
      throw new Error(`Unexpected dependency ${name}`)
    },
  })
  return {
    requests,
    render() {
      cursor = 0
      const result = exports.useConversations()
      effects.splice(0).forEach(effect => effect())
      return result
    },
    unmount() { slots.forEach(slot => slot?.cleanup?.()) },
  }
}

const settle = () => new Promise(resolve => setImmediate(resolve))
const resolveDetail = (request, id, title = String(id)) => request.resolve({ code: 0, data: { id, title, cluster_id: id, messages: [] } })

test('late A response cannot replace B or its cluster', async () => {
  const h = harness()
  let hook = h.render()
  hook.selectConversation(1)
  hook = h.render()
  hook.selectConversation(2)
  hook = h.render()
  assert.equal(hook.activeConversation, null)
  resolveDetail(h.requests[1], 2)
  await settle()
  assert.equal(h.render().activeConversation.id, 2)
  resolveDetail(h.requests[0], 1)
  await settle()
  hook = h.render()
  assert.equal(hook.activeConversation.id, 2)
  assert.equal(hook.activeConversation.cluster_id, 2)
  hook.selectConversation(3)
  assert.equal(h.render().activeConversation, null)
})

test('latest refresh wins and background conversation refresh is ignored', async () => {
  const h = harness()
  let hook = h.render()
  hook.selectConversation(1)
  hook = h.render()
  const latest = hook.fetchConversationDetail(1)
  await hook.fetchConversationDetail(2)
  assert.equal(h.requests.length, 2)
  resolveDetail(h.requests[1], 1, 'new')
  await latest
  resolveDetail(h.requests[0], 1, 'old')
  await settle()
  assert.equal(h.render().activeConversation.title, 'new')
})

test('unmount invalidates a pending response', async () => {
  const h = harness()
  let hook = h.render()
  hook.selectConversation(1)
  h.render()
  h.unmount()
  resolveDetail(h.requests[0], 1)
  await settle()
  assert.equal(h.render().activeConversation, null)
})
