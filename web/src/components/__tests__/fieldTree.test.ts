import { describe, expect, it } from 'vitest'
import {
  cascaderPathToSegments, fieldTreeToCascaderOptions, findGroupVersionForResource, resolveLeafType,
  type FieldNode,
} from '../fieldTree'

const podFields: FieldNode[] = [
  {
    name: 'spec', type: 'object', isArray: false, isMap: false,
    children: [
      {
        name: 'containers', type: 'array', isArray: true, isMap: false,
        children: [
          { name: 'image', type: 'string', isArray: false, isMap: false },
          {
            name: 'securityContext', type: 'object', isArray: false, isMap: false,
            children: [{ name: 'privileged', type: 'boolean', isArray: false, isMap: false }],
          },
        ],
      },
    ],
  },
  {
    name: 'metadata', type: 'object', isArray: false, isMap: false,
    children: [
      { name: 'labels', type: 'object', isArray: false, isMap: true, mapValueType: 'string' },
      { name: 'annotations', type: 'object', isArray: false, isMap: true },
    ],
  },
]

describe('fieldTreeToCascaderOptions', () => {
  it('maps name to value/label and preserves metadata', () => {
    const options = fieldTreeToCascaderOptions(podFields)
    expect(options[0]).toMatchObject({ value: 'spec', label: 'spec', type: 'object', isArray: false })
  })

  it('drops children for map fields even if the backend sent none', () => {
    const options = fieldTreeToCascaderOptions(podFields)
    const metadata = options.find((o) => o.value === 'metadata')!
    const labels = metadata.children!.find((o) => o.value === 'labels')!
    expect(labels.isMap).toBe(true)
    expect(labels.children).toBeUndefined()
  })
})

describe('cascaderPathToSegments', () => {
  it('resolves a plain nested path', () => {
    const options = fieldTreeToCascaderOptions(podFields)
    const segments = cascaderPathToSegments(['spec', 'containers', 'securityContext', 'privileged'], options)
    expect(segments).toEqual([
      { field: 'spec', isArray: false, isMap: false },
      { field: 'containers', isArray: true, isMap: false },
      { field: 'securityContext', isArray: false, isMap: false },
      { field: 'privileged', isArray: false, isMap: false },
    ])
  })

  it('throws for an unknown segment', () => {
    const options = fieldTreeToCascaderOptions(podFields)
    expect(() => cascaderPathToSegments(['spec', 'nope'], options)).toThrow()
  })
})

describe('resolveLeafType', () => {
  it('returns the type of the last segment along the path', () => {
    const options = fieldTreeToCascaderOptions(podFields)
    expect(resolveLeafType(['spec', 'containers', 'securityContext', 'privileged'], options)).toBe('boolean')
  })

  it('returns the map value type (not the hardcoded object type) for a map field', () => {
    const options = fieldTreeToCascaderOptions(podFields)
    expect(resolveLeafType(['metadata', 'labels'], options)).toBe('string')
  })

  it('falls back to string for a map field with no mapValueType from the backend', () => {
    const options = fieldTreeToCascaderOptions(podFields)
    expect(resolveLeafType(['metadata', 'annotations'], options)).toBe('string')
  })
})

describe('findGroupVersionForResource', () => {
  const resources = [
    { apiGroups: [''], apiVersions: ['v1'], resources: ['pods', 'services'] },
    { apiGroups: ['apps'], apiVersions: ['v1'], resources: ['deployments'] },
  ]

  it('finds the group/version for a resource plural name', () => {
    expect(findGroupVersionForResource(resources, 'deployments')).toEqual({ group: 'apps', version: 'v1' })
    expect(findGroupVersionForResource(resources, 'pods')).toEqual({ group: '', version: 'v1' })
  })

  it('returns null for a resource not covered by any match entry', () => {
    expect(findGroupVersionForResource(resources, 'secrets')).toBeNull()
  })
})
