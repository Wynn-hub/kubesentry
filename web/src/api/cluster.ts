import { request } from './http'

export interface ClusterListItem {
  name: string
  phase: string
  operatorVersion: string
  lastHeartbeatAt: string | null
  createdAt: string
}

export interface ClusterDetail {
  name: string
  clusterUid: string
  phase: string
  operatorVersion: string
  lastHeartbeatAt: string | null
  labels: Record<string, string> | null
  createdAt: string
  updatedAt: string
}

export interface CreateClusterRequest {
  name: string
  labels?: Record<string, string>
}

export interface CreateClusterResponse {
  name: string
  token: string
}

export const listClusters = () => request<ClusterListItem[]>('GET', '/clusters')
export const getCluster = (name: string) => request<ClusterDetail>('GET', `/clusters/${name}`)
export const createCluster = (req: CreateClusterRequest) =>
  request<CreateClusterResponse>('POST', '/clusters', req)
export const updateClusterLabels = (name: string, labels: Record<string, string>) =>
  request<null>('PUT', `/clusters/${name}`, { labels })
export const deleteCluster = (name: string) => request<null>('DELETE', `/clusters/${name}`)
export const reissueToken = (name: string) =>
  request<CreateClusterResponse>('POST', `/clusters/${name}/reissue-token`)
