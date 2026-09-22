import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type ExceptionMatch = components['schemas']['dto.ExceptionMatchInput']
export type ExceptionListItem = components['schemas']['dto.ExceptionItem']
export type ExceptionSpec = components['schemas']['dto.SwagExceptionSpec']
export type ExceptionStatus = components['schemas']['dto.ExceptionStatusView']
export type ExceptionDetail = components['schemas']['dto.SwagExceptionDetail']
export type ExceptionRequest = components['schemas']['dto.SwagExceptionInput']
export type ListExceptionsQuery = NonNullable<operations['listExceptions']['parameters']['query']>

export const listExceptions = (params: ListExceptionsQuery = {}) =>
  extract<'listExceptions'>(apiClient.GET('/exceptions', { params: { query: params } }))
export const getException = (name: string) =>
  extract<'getException'>(apiClient.GET('/exceptions/{name}', { params: { path: { name } } }))
export const createException = (body: ExceptionRequest) =>
  extract<'createException'>(apiClient.POST('/exceptions', { body }))
export const updateException = (name: string, body: ExceptionRequest) =>
  extract<'updateException'>(apiClient.PUT('/exceptions/{name}', { params: { path: { name } }, body }))
export const deleteException = (name: string) =>
  extract<'deleteException'>(apiClient.DELETE('/exceptions/{name}', { params: { path: { name } } }))
