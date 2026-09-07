import fs from 'node:fs'
import { createWorkspaceStore } from '@scalar/workspace-store/client'

const input = process.argv[2]
if (!input) throw new Error('usage: check-scalar-openapi.mjs <openapi.json>')

const document = JSON.parse(fs.readFileSync(input, 'utf8'))
const store = createWorkspaceStore()
const loaded = await store.addDocument({ name: 'console', document })
const active = store.workspace.activeDocument

if (!loaded || !active?.openapi || !Object.keys(active.paths ?? {}).length) {
  throw new Error('Scalar failed to load the OpenAPI document')
}

console.log(`Scalar loaded ${active.openapi} with ${Object.keys(active.paths).length} path(s)`)
