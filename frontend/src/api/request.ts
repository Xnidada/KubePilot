import axios, { AxiosInstance, AxiosRequestConfig, AxiosResponse } from 'axios'
import { message } from 'antd'
import { useAuthStore } from '../stores/auth'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

const instance: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 180000,  // 180秒，AI 操作可能需要较长时间
  headers: {
    'Content-Type': 'application/json',
  },
})

instance.interceptors.request.use(
  (config) => {
    const token = useAuthStore.getState().token
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

instance.interceptors.response.use(
  (response: AxiosResponse) => {
    const { data } = response
    if (data.code !== 0 && data.code !== undefined) {
      message.error(data.message || '请求失败')
      return Promise.reject(new Error(data.message))
    }
    return response
  },
  (error) => {
    if (error.response) {
      const { status, data } = error.response
      switch (status) {
        case 401:
          message.error('登录已过期，请重新登录')
          useAuthStore.getState().logout()
          window.location.href = '/login'
          break
        case 403:
          message.error('没有权限访问')
          break
        case 404:
          message.error('请求的资源不存在')
          break
        case 500:
          message.error('服务器错误')
          break
        default:
          message.error(data?.message || '请求失败')
      }
    } else {
      message.error('网络错误')
    }
    return Promise.reject(error)
  }
)

export const get = <T>(url: string, config?: AxiosRequestConfig): Promise<T> => {
  return instance.get(url, config).then((res) => res.data)
}

export const post = <T>(url: string, data?: any, config?: AxiosRequestConfig): Promise<T> => {
  return instance.post(url, data, config).then((res) => res.data)
}

export const put = <T>(url: string, data?: any, config?: AxiosRequestConfig): Promise<T> => {
  return instance.put(url, data, config).then((res) => res.data)
}

export const del = <T>(url: string, config?: AxiosRequestConfig): Promise<T> => {
  return instance.delete(url, config).then((res) => res.data)
}

export default instance
