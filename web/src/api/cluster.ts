import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type ClusterListItem = components['schemas']['dto.ClusterItem']
export type ClusterDetail = components['schemas']['dto.ClusterView']
export type ImageParams = components['schemas']['dto.ClusterImageInput']
export type InstallParams = components['schemas']['dto.ClusterInstallInput']
export type InstallRecordItem = components['schemas']['dto.InstallRecordItemView']
export type CreateClusterRequest = components['schemas']['dto.ClusterCreateInput']
export type CreateClusterResponse = components['schemas']['dto.ClusterCommandView']
export type UninstallTask = components['schemas']['dto.UninstallTaskHTTPView']
export type ListClustersQuery = NonNullable<operations['listClusters']['parameters']['query']>
export type ListInstallRecordsQuery = NonNullable<operations['listInstallRecords']['parameters']['query']>

export const DEFAULT_INSTALL_PARAMS: InstallParams = {
  namespace: 'kubesentry-system',
  webhookImage: { repository: 'wynnhub/kubesentry-webhook', tag: 'latest', pullPolicy: 'IfNotPresent' },
  operatorImage: { repository: 'wynnhub/kubesentry-operator', tag: 'latest', pullPolicy: 'IfNotPresent' },
  highAvailability: false,
  gatewayAddress: '',
}

export const listClusters = (params: ListClustersQuery = {}) =>
  extract<'listClusters'>(apiClient.GET('/clusters', { params: { query: params } }))
export const getCluster = (name: string) =>
  extract<'getCluster'>(apiClient.GET('/clusters/{name}', { params: { path: { name } } }))
export const createCluster = (body: CreateClusterRequest) =>
  extract<'createCluster'>(apiClient.POST('/clusters', { body }))
export const updateClusterLabels = (name: string, labels: components['schemas']['dto.ClusterLabelsInput']['labels']) =>
  extract<'updateCluster'>(apiClient.PUT('/clusters/{name}', { params: { path: { name } }, body: { labels } }))
export const deleteCluster = (name: string) =>
  extract<'deleteCluster'>(apiClient.DELETE('/clusters/{name}', { params: { path: { name } } }))
export const reissueToken = (name: string, installParams: InstallParams) =>
  extract<'reissueClusterToken'>(apiClient.POST('/clusters/{name}/reissue-token', { params: { path: { name } }, body: { installParams } }))
export const getLatestInstallParams = (name: string) =>
  extract<'getLatestInstallParams'>(apiClient.GET('/clusters/{name}/latest-install-params', { params: { path: { name } } }))
export const uninstallCluster = (name: string) =>
  extract<'createUninstallTask'>(apiClient.POST('/clusters/{name}/uninstall', { params: { path: { name } } }))
export const listUninstallTasks = (name: string) =>
  extract<'listUninstallTasks'>(apiClient.GET('/clusters/{name}/uninstall-tasks', { params: { path: { name } } }))
export const retryUninstallTask = (name: string, taskID: string) =>
  extract<'retryUninstallTask'>(apiClient.POST('/clusters/{name}/uninstall-tasks/{taskID}/retry', { params: { path: { name, taskID } } }))
export const listInstallRecords = (params: ListInstallRecordsQuery = {}) =>
  extract<'listInstallRecords'>(apiClient.GET('/install-records', { params: { query: params } }))
