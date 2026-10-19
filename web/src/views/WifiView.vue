<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { scanWifi, connectWifi, getInterfaces } from '../api'

const props = defineProps({
  capabilities: { type: Object, default: null }
})

const networks = ref([])
const loading = ref(false)
const interfaces = ref([])
const scanDenied = ref(false)

const dialogVisible = ref(false)
const target = ref(null)
const password = ref('')
const connecting = ref(false)

const scanSupported = computed(() => !props.capabilities || props.capabilities.wifi_scan !== false)

// 连接时使用的无线网卡名：优先已关联网络的无线网卡，取不到则为空
const wifiInterfaceName = computed(() => {
  const wifiList = interfaces.value.filter(item => item.type === 'wifi')
  const active = wifiList.find(item => item.ssid)
  if (active) return active.name
  return wifiList.length > 0 ? wifiList[0].name : ''
})

async function scan() {
  if (!scanSupported.value) return
  loading.value = true
  try {
    const data = await scanWifi()
    networks.value = Array.isArray(data) ? data : []
    scanDenied.value = false
  } catch (e) {
    if (e.code === 'permission_denied') {
      scanDenied.value = true
    } else {
      scanDenied.value = false
      ElMessage.error(e.message)
    }
  } finally {
    loading.value = false
  }
}

async function loadInterfaces() {
  try {
    const data = await getInterfaces()
    interfaces.value = Array.isArray(data) ? data : []
  } catch (e) {
    // 网卡列表仅用于确定连接用的网卡名，失败时以空字符串提交
  }
}

function openConnect(row) {
  target.value = row
  password.value = ''
  dialogVisible.value = true
}

async function handleConnect() {
  if (!target.value) return
  connecting.value = true
  try {
    await connectWifi({
      interface: wifiInterfaceName.value,
      ssid: target.value.ssid,
      password: password.value
    })
    ElMessage.success('连接成功')
    dialogVisible.value = false
    await scan()
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    connecting.value = false
  }
}

onMounted(() => {
  loadInterfaces()
  scan()
})
</script>

<template>
  <section class="block">
    <div class="block__header">
      <div>
        <h2 class="block__title">Wi-Fi 网络</h2>
        <p class="block__desc">扫描附近的无线网络并选择一个连接</p>
      </div>
      <div class="block__actions">
        <span v-if="wifiInterfaceName" class="mono wifi-iface">网卡 {{ wifiInterfaceName }}</span>
        <el-button :icon="Refresh" :loading="loading" :disabled="!scanSupported" @click="scan">刷新</el-button>
      </div>
    </div>

    <el-alert
      v-if="!scanSupported"
      type="warning"
      :closable="false"
      title="当前系统不支持 Wi-Fi 扫描"
    />

    <el-alert
      v-else-if="scanDenied"
      type="warning"
      :closable="false"
      title="Wi-Fi 扫描需要定位授权"
      description="服务端已尝试弹出系统定位授权窗口，请在弹窗中点击允许后重新扫描；若未出现弹窗，请在系统设置的隐私与安全性中为终端 App 开启定位服务后再试。"
    />

    <el-table v-else v-loading="loading" :data="networks" border>
      <el-table-column label="SSID" min-width="160">
        <template #default="{ row }">
          <span class="mono">{{ row.ssid }}</span>
        </template>
      </el-table-column>
      <el-table-column label="信号" min-width="170">
        <template #default="{ row }">
          <div class="wifi-signal">
            <el-progress :percentage="row.signal || 0" :stroke-width="6" :show-text="false" class="wifi-signal__bar" />
            <span class="wifi-signal__num">{{ row.signal || 0 }}%</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="安全性" min-width="120">
        <template #default="{ row }">
          <span v-if="row.security">{{ row.security }}</span>
          <span v-else class="text-dim">开放</span>
        </template>
      </el-table-column>
      <el-table-column label="信道" width="80">
        <template #default="{ row }">
          <span class="mono">{{ row.channel || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="当前" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.current" type="success" size="small">当前</el-tag>
          <span v-else class="text-dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="80" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openConnect(row)">连接</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      :model-value="dialogVisible"
      :title="`连接 ${target ? target.ssid : ''}`"
      width="440px"
      @update:model-value="value => (dialogVisible = value)"
    >
      <el-form label-width="70px" @submit.prevent>
        <el-form-item label="密码">
          <el-input
            v-model="password"
            type="password"
            show-password
            autocomplete="off"
            placeholder="开放网络无需填写密码"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="connecting" @click="handleConnect">连接</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.wifi-iface {
  font-size: 13px;
  color: var(--color-text-dim);
}

.wifi-signal {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.wifi-signal__bar {
  flex: 1;
  min-width: 80px;
}

.wifi-signal__num {
  width: 38px;
  text-align: right;
  color: var(--color-text-secondary);
  font-family: var(--font-mono);
  font-size: 13px;
}
</style>
