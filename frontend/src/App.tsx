import { Routes, Route, Navigate } from 'react-router-dom'
import MainLayout from './layouts/MainLayout'
import Login from './pages/Login'
import Dashboard from './pages/dashboard/Index'
import ClusterList from './pages/cluster/List'
import ClusterDetail from './pages/cluster/Detail'
import WorkloadDeployments from './pages/workload/Deployments'
import CreateDeployment from './pages/workload/CreateDeployment'
import WorkloadPods from './pages/workload/Pods'
import WorkloadServices from './pages/workload/Services'
import PersistentVolumes from './pages/storage/PersistentVolumes'
import PersistentVolumeClaims from './pages/storage/PersistentVolumeClaims'
import MonitorOverview from './pages/monitor/Overview'
import MonitorDashboard from './pages/monitor/Dashboard'
import AppStoreList from './pages/appstore/List'
import SystemUsers from './pages/system/Users'
import SystemRoles from './pages/system/Roles'
import { useAuthStore } from './stores/auth'

function PrivateRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated)
  return isAuthenticated ? <>{children}</> : <Navigate to="/login" replace />
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <PrivateRoute>
            <MainLayout />
          </PrivateRoute>
        }
      >
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<Dashboard />} />
        <Route path="clusters" element={<ClusterList />} />
        <Route path="clusters/:id" element={<ClusterDetail />} />
        <Route path="workloads/deployments" element={<WorkloadDeployments />} />
        <Route path="workloads/deployments/create" element={<CreateDeployment />} />
        <Route path="workloads/pods" element={<WorkloadPods />} />
        <Route path="workloads/services" element={<WorkloadServices />} />
        <Route path="storage/pvs" element={<PersistentVolumes />} />
        <Route path="storage/pvcs" element={<PersistentVolumeClaims />} />
        <Route path="monitor" element={<MonitorOverview />} />
        <Route path="monitor/dashboard" element={<MonitorDashboard />} />
        <Route path="appstore" element={<AppStoreList />} />
        <Route path="system/users" element={<SystemUsers />} />
        <Route path="system/roles" element={<SystemRoles />} />
      </Route>
    </Routes>
  )
}

export default App
