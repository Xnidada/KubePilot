import { get, post, del } from './request'

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

export interface CreatePVCRequest {
  namespace: string
  name: string
  capacity: string
  access_modes: string[]
  storage_class?: string
  volume_name?: string
}

// PV
export const getPVs = (clusterId: number) => {
  return get<{ code: number; data: PV[] }>(`/clusters/${clusterId}/workloads/pvs`)
}

export const createPV = (clusterId: number, data: CreatePVRequest) => {
  return post(`/clusters/${clusterId}/workloads/pvs`, data)
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

export const deletePVC = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/pvcs/${namespace}/${name}`)
}

// StorageClass
export const getStorageClasses = (clusterId: number) => {
  return get<{ code: number; data: StorageClass[] }>(`/clusters/${clusterId}/workloads/storageclasses`)
}
