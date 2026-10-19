<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getVersion } from './api'
import InterfacesView from './views/InterfacesView.vue'
import WifiView from './views/WifiView.vue'
import HostsView from './views/HostsView.vue'
import ProxyView from './views/ProxyView.vue'

const version = ref(null)
const ready = ref(false)
const activeTab = ref('interfaces')

const capabilities = computed(() => (version.value && version.value.capabilities) || null)

// commit 为 none 或为空时不展示，避免误导
const shortCommit = computed(() => {
  const commit = version.value && version.value.commit
  if (!commit || commit === 'none') return ''
  return commit.length > 7 ? commit.slice(0, 7) : commit
})

onMounted(async () => {
  try {
    version.value = await getVersion()
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    ready.value = true
  }
})
</script>

<template>
  <div class="app">
    <header class="app-header">
      <div class="app-header__inner">
        <a
          class="app-header__brand"
          href="https://github.com/LinuxSuRen/netcfg"
          target="_blank"
          rel="noopener"
        >
          <img class="app-header__logo" src="/logo.svg" alt="netcfg logo" />
          netcfg
        </a>
        <span v-if="version" class="app-header__meta">
          版本 {{ version.version }} · {{ version.os }}/{{ version.arch }}<template v-if="shortCommit"> · {{ shortCommit }}</template>
        </span>
      </div>
    </header>

    <main class="page">
      <el-tabs v-if="ready" v-model="activeTab" class="app-tabs">
        <el-tab-pane label="网卡" name="interfaces">
          <InterfacesView :capabilities="capabilities" />
        </el-tab-pane>
        <el-tab-pane label="Wi-Fi" name="wifi" lazy>
          <WifiView :capabilities="capabilities" />
        </el-tab-pane>
        <el-tab-pane label="hosts" name="hosts" lazy>
          <HostsView />
        </el-tab-pane>
        <el-tab-pane label="代理" name="proxy" lazy>
          <ProxyView :capabilities="capabilities" />
        </el-tab-pane>
      </el-tabs>
    </main>
  </div>
</template>

<style scoped>
.app-header {
  background: var(--color-bg-panel);
  border-bottom: 1px solid var(--color-border-subtle);
}

.app-header__inner {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--space-3) var(--space-6);
  display: flex;
  align-items: baseline;
  gap: var(--space-3);
}

.app-header__brand {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: 18px;
  font-weight: 600;
  color: var(--color-text-primary);
  font-family: var(--font-display);
  text-decoration: none;
}

.app-header__brand:hover {
  opacity: 0.85;
}

.app-header__logo {
  width: 22px;
  height: 22px;
  border-radius: 5px;
}

.app-header__meta {
  font-size: 13px;
  color: var(--color-text-dim);
  font-family: var(--font-mono);
}
</style>
