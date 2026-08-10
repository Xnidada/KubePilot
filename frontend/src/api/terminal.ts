import { post } from './request'

interface WebSocketTicketResponse {
  code: number
  data: {
    ticket: string
    expires_in: number
  }
}

export const createPodTerminalTicket = (
  clusterId: number,
  namespace: string,
  podName: string,
) => {
  return post<WebSocketTicketResponse>(
    `/ws/tickets/pod/${clusterId}/${encodeURIComponent(namespace)}/${encodeURIComponent(podName)}`,
  )
}

export const createNodeShellTicket = (clusterId: number, nodeName: string) => {
  return post<WebSocketTicketResponse>(
    `/ws/tickets/node/${clusterId}/${encodeURIComponent(nodeName)}`,
  )
}
