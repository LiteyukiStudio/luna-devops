import { describe, expect, it } from 'vitest'
import { parseFamiliarImageRef } from './image-ref'

describe('parseFamiliarImageRef', () => {
  it('normalizes official and namespaced Docker Hub references to familiar form', () => {
    expect(parseFamiliarImageRef('memcached:1.6-alpine')).toEqual({
      full: 'memcached:1.6-alpine',
      repository: 'memcached',
      suffix: ':1.6-alpine',
    })

    expect(parseFamiliarImageRef('docker.io/library/memcached:1.6-alpine')).toEqual({
      full: 'memcached:1.6-alpine',
      repository: 'memcached',
      suffix: ':1.6-alpine',
    })

    expect(parseFamiliarImageRef('index.docker.io/typesense/typesense:28.0')).toEqual({
      full: 'typesense/typesense:28.0',
      repository: 'typesense/typesense',
      suffix: ':28.0',
    })
  })

  it('keeps registry domain and port for non-Docker Hub images while separating the tag suffix', () => {
    expect(parseFamiliarImageRef('ghcr.io/usememos/memos:stable')).toEqual({
      full: 'ghcr.io/usememos/memos:stable',
      repository: 'ghcr.io/usememos/memos',
      suffix: ':stable',
    })

    expect(parseFamiliarImageRef('registry.example.com:8443/team/thisisanlong-service:latest')).toEqual({
      full: 'registry.example.com:8443/team/thisisanlong-service:latest',
      repository: 'registry.example.com:8443/team/thisisanlong-service',
      suffix: ':latest',
    })
  })

  it('fills fallback or default latest tag when untagged and shortens digest-only references', () => {
    expect(parseFamiliarImageRef('localhost:5000/worker', 'v1.4.0')).toEqual({
      full: 'localhost:5000/worker:v1.4.0',
      repository: 'localhost:5000/worker',
      suffix: ':v1.4.0',
    })

    expect(parseFamiliarImageRef('redis')).toEqual({
      full: 'redis:latest',
      repository: 'redis',
      suffix: ':latest',
    })

    expect(parseFamiliarImageRef('ghcr.io/org/app@sha256:0123456789abcdef0123456789abcdef')).toEqual({
      full: 'ghcr.io/org/app@sha256:0123456789abcdef0123456789abcdef',
      repository: 'ghcr.io/org/app',
      suffix: '@sha256:0123456…',
    })
  })
})
