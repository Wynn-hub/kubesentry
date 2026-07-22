import { request } from './http'
import type { ListQuery, PageResult } from './pagination'

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

export interface ImageParams {
  repository: string
  tag: string
  pullPolicy: string
}

export interface InstallParams {
  namespace: string
  webhookImage: ImageParams
  operatorImage: ImageParams
  gatewayAddress: string
  gatewayTlsServerName?: string
  gatewayCaBundle?: string
}

export const DEFAULT_INSTALL_PARAMS: InstallParams = {
  namespace: 'kubesentry-system',
  webhookImage: { repository: 'wynnhub/kubesentry-webhook', tag: 'latest', pullPolicy: 'IfNotPresent' },
  operatorImage: { repository: 'wynnhub/kubesentry-operator', tag: 'latest', pullPolicy: 'IfNotPresent' },
  gatewayAddress: '',
}

export interface InstallRecordItem {
  clusterName: string
  namespace: string
  webhookRepository: string
  webhookTag: string
  operatorRepository: string
  operatorTag: string
  gatewayAddress: string
  createdAt: string
}

export interface CreateClusterRequest {
  name: string
  labels?: Record<string, string>
  installParams: InstallParams
}

export interface CreateClusterResponse {
  name: string
  command: string
}

export const listClusters = (params: ListQuery = {}) =>
  request<PageResult<ClusterListItem>>('GET', '/clusters', undefined, params)
export const getCluster = (name: string) => request<ClusterDetail>('GET', `/clusters/${name}`)
export const createCluster = (req: CreateClusterRequest) =>
  request<CreateClusterResponse>('POST', '/clusters', req)
export const updateClusterLabels = (name: string, labels: Record<string, string>) =>
  request<null>('PUT', `/clusters/${name}`, { labels })
export const deleteCluster = (name: string) => request<null>('DELETE', `/clusters/${name}`)
export const reissueToken = (name: string, installParams: InstallParams) =>
  request<CreateClusterResponse>('POST', `/clusters/${name}/reissue-token`, { installParams })
export const getLatestInstallParams = (name: string) =>
  request<InstallParams | null>('GET', `/clusters/${name}/latest-install-params`)
export const listInstallRecords = (params: ListQuery = {}) =>
  request<PageResult<InstallRecordItem>>('GET', '/install-records', undefined, params)
