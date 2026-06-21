import { get, post, put, del } from './request'

export interface PV {
  name: string
  capacity: string
  access_modes: string
  reclaim_policy: string
  status: string
  claim: string
  storage_class: string
  age: string
}

export interface PVC {
  name: string
  namespace: string
  status: string
  volume: string
  capacity: string
  access_modes: string
  storage_class: string
  age: string
}

export interface StorageClass {
  name: string
  provisioner: string
  reclaim_policy: string
  volume_binding_mode: string
  age: string
}

export interface CreatePVRequest {
  name: string
  capacity: string
  access_modes: string[]
  reclaim_policy?: string
  storage_class?: string
  host_path?: string
  nfs?: {
    server: string
    path: string
  }
}

export interface UpdatePVRequest {
  capacity?: string
  access_modes?: string[]
  reclaim_policy?: string
  storage_class?: string
}

export interface CreatePVCRequest {
  namespace: string
  name: string
  capacity: string
  access_modes: string[]
  storage_class?: string
  volume_name?: string
}

export interface UpdatePVCRequest {
  capacity?: string
  access_modes?: string[]
  storage_class?: string
}

export interface CreateStorageClassRequest {
  name: string
  provisioner: string
  reclaim_policy?: string
  volume_binding_mode?: string
  parameters?: Record<string, string>
}

export interface UpdateStorageClassRequest {
  provisioner?: string
  reclaim_policy?: string
  volume_binding_mode?: string
  parameters?: Record<string, string>
}

// PV
export const getPVs = (clusterId: number) => {
  return get<{ code: number; data: PV[] }>(`/clusters/${clusterId}/workloads/pvs`)
}

export const createPV = (clusterId: number, data: CreatePVRequest) => {
  return post(`/clusters/${clusterId}/workloads/pvs`, data)
}

export const updatePV = (clusterId: number, name: string, data: UpdatePVRequest) => {
  return put(`/clusters/${clusterId}/workloads/pvs/${name}`, data)
}

export const deletePV = (clusterId: number, name: string) => {
  return del(`/clusters/${clusterId}/workloads/pvs/${name}`)
}

// PVC
export const getPVCs = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: PVC[] }>(`/clusters/${clusterId}/workloads/pvcs`, { params })
}

export const createPVC = (clusterId: number, data: CreatePVCRequest) => {
  return post(`/clusters/${clusterId}/workloads/pvcs`, data)
}

export const updatePVC = (clusterId: number, namespace: string, name: string, data: UpdatePVCRequest) => {
  return put(`/clusters/${clusterId}/workloads/pvcs/${namespace}/${name}`, data)
}

export const deletePVC = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/pvcs/${namespace}/${name}`)
}

// StorageClass
export const listStorageClasses = (clusterId: number) => {
  return get<{ code: number; data: StorageClass[] }>(`/clusters/${clusterId}/workloads/storageclasses`)
}

export const createStorageClass = (clusterId: number, data: CreateStorageClassRequest) => {
  return post(`/clusters/${clusterId}/workloads/storageclasses`, data)
}

export const updateStorageClass = (clusterId: number, name: string, data: UpdateStorageClassRequest) => {
  return put(`/clusters/${clusterId}/workloads/storageclasses/${name}`, data)
}

export const deleteStorageClass = (clusterId: number, name: string) => {
  return del(`/clusters/${clusterId}/workloads/storageclasses/${name}`)
}

// 兼容旧接口
export const getStorageClasses = listStorageClasses
