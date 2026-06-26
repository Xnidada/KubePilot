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
import ConfigMaps from './pages/workload/ConfigMaps'
import Secrets from './pages/workload/Secrets'
import Ingresses from './pages/workload/Ingresses'
import Namespaces from './pages/workload/Namespaces'
import StatefulSets from './pages/workload/StatefulSets'
import DaemonSets from './pages/workload/DaemonSets'
import Jobs from './pages/workload/Jobs'
import CronJobs from './pages/workload/CronJobs'
import ReplicaSets from './pages/workload/ReplicaSets'
import CRDs from './pages/workload/CRDs'
import PersistentVolumes from './pages/storage/PersistentVolumes'
import PersistentVolumeClaims from './pages/storage/PersistentVolumeClaims'
import StorageClasses from './pages/storage/StorageClasses'
import MonitorOverview from './pages/monitor/Overview'
import MonitorDashboard from './pages/monitor/Dashboard'
import AppStoreList from './pages/appstore/List'
import AIChat from './pages/aiops/Chat'
import AIDiagnosis from './pages/aiops/Diagnosis'
import AISettings from './pages/aiops/Settings'
import AIAgent from './pages/aiops/Agent'
import AITools from './pages/aiops/AITools'
import SystemUsers from './pages/system/Users'
import SystemRoles from './pages/system/Roles'
import TwoFactorAuth from './pages/system/TwoFactorAuth'
import Profile from './pages/Profile'
import Inspection from './pages/cluster/Inspection'
import EventForward from './pages/cluster/EventForward'
import SchedulerTasks from './pages/scheduler/Tasks'
import SchedulerQueues from './pages/scheduler/Queues'
import HPAs from './pages/workload/HPAs'
import ResourceCompare from './pages/workload/ResourceCompare'
import NodePressurePage from './pages/ops/NodePressure'
import EventTimelinePage from './pages/ops/EventTimeline'
import ResourceGraphPage from './pages/ops/ResourceGraph'
import IdleResourcesPage from './pages/ops/IdleResources'
import NetworkPolicies from './pages/workload/NetworkPolicies'
import ResourceCost from './pages/monitor/ResourceCost'
import YAMLDiff from './pages/workload/YAMLDiff'
import EnvClone from './pages/workload/EnvClone'
import GPUScheduling from './pages/workload/GPUScheduling'
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
        <Route path="workloads/configmaps" element={<ConfigMaps />} />
        <Route path="workloads/secrets" element={<Secrets />} />
        <Route path="workloads/ingresses" element={<Ingresses />} />
        <Route path="workloads/namespaces" element={<Namespaces />} />
        <Route path="workloads/statefulsets" element={<StatefulSets />} />
        <Route path="workloads/daemonsets" element={<DaemonSets />} />
        <Route path="workloads/jobs" element={<Jobs />} />
        <Route path="workloads/cronjobs" element={<CronJobs />} />
        <Route path="workloads/replicasets" element={<ReplicaSets />} />
        <Route path="workloads/crds" element={<CRDs />} />
        <Route path="storage/pvs" element={<PersistentVolumes />} />
        <Route path="storage/pvcs" element={<PersistentVolumeClaims />} />
        <Route path="storage/storageclasses" element={<StorageClasses />} />
        <Route path="monitor" element={<MonitorOverview />} />
        <Route path="monitor/dashboard" element={<MonitorDashboard />} />
        <Route path="appstore" element={<AppStoreList />} />
        <Route path="aiops/chat" element={<AIChat />} />
        <Route path="aiops/agent" element={<AIAgent />} />
        <Route path="aiops/diagnosis" element={<AIDiagnosis />} />
        <Route path="aiops/tools" element={<AITools />} />
        <Route path="aiops/settings" element={<AISettings />} />
        <Route path="system/users" element={<SystemUsers />} />
        <Route path="system/roles" element={<SystemRoles />} />
        <Route path="system/2fa" element={<TwoFactorAuth />} />
        <Route path="profile" element={<Profile />} />
        <Route path="cluster/inspection" element={<Inspection />} />
        <Route path="cluster/event-forward" element={<EventForward />} />
        <Route path="scheduler/tasks" element={<SchedulerTasks />} />
        <Route path="scheduler/queues" element={<SchedulerQueues />} />
        <Route path="workloads/hpas" element={<HPAs />} />
        <Route path="workloads/compare" element={<ResourceCompare />} />
        <Route path="monitor/node-pressure" element={<NodePressurePage />} />
        <Route path="ops/events" element={<EventTimelinePage />} />
        <Route path="ops/resource-graph" element={<ResourceGraphPage />} />
        <Route path="ops/idle-resources" element={<IdleResourcesPage />} />
        <Route path="workloads/networkpolicies" element={<NetworkPolicies />} />
        <Route path="monitor/cost" element={<ResourceCost />} />
        <Route path="workloads/yaml-diff" element={<YAMLDiff />} />
        <Route path="workloads/env-clone" element={<EnvClone />} />
        <Route path="workloads/gpu" element={<GPUScheduling />} />
      </Route>
    </Routes>
  )
}

export default App
