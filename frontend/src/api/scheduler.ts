import { get, post, put, del } from './request'

// ==================== 队列管理 ====================

export interface TaskQueue {
  id: number
  name: string
  display_name: string
  description: string
  priority: number
  weight: number
  max_cpu: string
  max_memory: string
  max_gpu: number
  max_tasks: number
  policy: string
  preemption: boolean
  status: string
  running_tasks: number
  pending_tasks: number
  created_at: string
}

export interface CreateQueueRequest {
  name: string
  display_name?: string
  description?: string
  priority?: number
  weight?: number
  max_cpu?: string
  max_memory?: string
  max_gpu?: number
  max_tasks?: number
  policy?: string
  preemption?: boolean
}

export const listQueues = () => {
  return get<{ code: number; data: TaskQueue[] }>('/scheduler/queues')
}

export const createQueue = (data: CreateQueueRequest) => {
  return post<{ code: number; data: TaskQueue }>('/scheduler/queues', data)
}

export const getQueue = (id: number) => {
  return get<{ code: number; data: { queue: TaskQueue; running_tasks: number; pending_tasks: number; total_tasks: number } }>(`/scheduler/queues/${id}`)
}

export const updateQueue = (id: number, data: Partial<CreateQueueRequest>) => {
  return put(`/scheduler/queues/${id}`, data)
}

export const deleteQueue = (id: number) => {
  return del(`/scheduler/queues/${id}`)
}

// ==================== 任务管理 ====================

export interface Task {
  id: number
  task_id: string
  name: string
  user_id: number
  queue_id: number
  cluster_id: number
  task_type: string
  priority: number
  cpu: string
  memory: string
  gpu: number
  gpu_type: string
  min_replicas: number
  replicas: number
  image: string
  command: string
  args: string
  env_vars: string
  namespace: string
  timeout: number
  max_retry: number
  retry_count: number
  status: string
  message: string
  k8s_job_name: string
  submitted_at: string
  started_at: string
  completed_at: string
  created_at: string
  queue?: TaskQueue
  user?: { id: number; username: string; real_name: string }
}

export interface TaskLog {
  id: number
  task_id: number
  level: string
  message: string
  created_at: string
}

export interface CreateTaskRequest {
  name: string
  queue_id: number
  cluster_id: number
  task_type: string
  priority?: number
  cpu?: string
  memory?: string
  gpu?: number
  gpu_type?: string
  replicas?: number
  min_replicas?: number
  image: string
  command?: string[]
  args?: string[]
  env_vars?: Record<string, string>
  namespace?: string
  timeout?: number
  max_retry?: number
}

export const listTasks = (params?: {
  page?: number
  size?: number
  queue_id?: number
  status?: string
}) => {
  return get<{ code: number; data: Task[]; total: number; page: number; size: number }>('/scheduler/tasks', { params })
}

export const createTask = (data: CreateTaskRequest) => {
  return post<{ code: number; data: Task }>('/scheduler/tasks', data)
}

export const getTask = (id: number) => {
  return get<{ code: number; data: { task: Task; logs: TaskLog[] } }>(`/scheduler/tasks/${id}`)
}

export const cancelTask = (id: number) => {
  return post(`/scheduler/tasks/${id}/cancel`)
}

export const retryTask = (id: number) => {
  return post(`/scheduler/tasks/${id}/retry`)
}

export const getTaskLogs = (id: number) => {
  return get<{ code: number; data: TaskLog[] }>(`/scheduler/tasks/${id}/logs`)
}

// ==================== 资源预留 ====================

export interface ResourceReservation {
  id: number
  name: string
  user_id: number
  queue_id: number
  cluster_id: number
  cpu: string
  memory: string
  gpu: number
  gpu_type: string
  start_time: string
  end_time: string
  recurring: boolean
  cron_expr: string
  node_name: string
  node_selector: string
  status: string
  created_at: string
}

export interface CreateReservationRequest {
  name: string
  queue_id: number
  cluster_id: number
  cpu?: string
  memory?: string
  gpu?: number
  gpu_type?: string
  start_time: string
  end_time: string
  recurring?: boolean
  cron_expr?: string
  node_name?: string
  node_selector?: Record<string, string>
}

export const listReservations = (params?: {
  user_id?: number
  status?: string
}) => {
  return get<{ code: number; data: ResourceReservation[] }>('/scheduler/reservations', { params })
}

export const createReservation = (data: CreateReservationRequest) => {
  return post<{ code: number; data: ResourceReservation }>('/scheduler/reservations', data)
}

export const deleteReservation = (id: number) => {
  return del(`/scheduler/reservations/${id}`)
}
