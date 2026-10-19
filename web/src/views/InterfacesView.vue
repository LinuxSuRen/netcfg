<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getInterfaces } from '../api'
import InterfaceDialog from '../components/InterfaceDialog.vue'

const props = defineProps({
  capabilities: { type: Object, default: null }
})

const TYPE_TEXT = {
  ethernet: '以太网',
  wifi: '无线',
  loopback: '回环',
  other: '其他'
}

const list = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref(null)

function ipv4Addresses(row) {
  const ips = Array.isArray(row.ips) ? row.ips : []
  return ips.filter(ip => ip.family === 'ipv4').map(ip => ip.address)
}

async function load() {
  loading.value = true
  try {
    const data = await getInterfaces()
    list.value = Array.isArray(data) ? data : []
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

function openConfig(row) {
  editing.value = row
  dialogVisible.value = true
}

onMounted(load)
</script>

<template>
  <section class="block">
    <div class="block__header">
      <div>
        <h2 class="block__title">网卡列表</h2>
        <p class="block__desc">查看本机网卡与地址分配情况，可对单个网卡配置地址和 DNS</p>
      </div>
      <div class="block__actions">
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="list" border>
      <el-table-column label="名称" min-width="170">
        <template #default="{ row }">
          <div class="mono">{{ row.name }}</div>
          <div v-if="row.hardware_addr" class="mono iface-mac">{{ row.hardware_addr }}</div>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="90">
        <template #default="{ row }">
          {{ TYPE_TEXT[row.type] || TYPE_TEXT.other }}
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.up ? 'success' : 'info'" size="small">{{ row.up ? '已启用' : '已停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="模式" width="80">
        <template #default="{ row }">
          {{ row.dhcp ? 'DHCP' : '静态' }}
        </template>
      </el-table-column>
      <el-table-column label="IPv4 地址" min-width="140">
        <template #default="{ row }">
          <div v-for="addr in ipv4Addresses(row)" :key="addr" class="mono">{{ addr }}</div>
          <span v-if="ipv4Addresses(row).length === 0" class="text-dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="网关" min-width="120">
        <template #default="{ row }">
          <span v-if="row.gateway" class="mono">{{ row.gateway }}</span>
          <span v-else class="text-dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="DNS" min-width="140">
        <template #default="{ row }">
          <div v-for="server in row.dns || []" :key="server" class="mono">{{ server }}</div>
          <span v-if="!row.dns || row.dns.length === 0" class="text-dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="80" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openConfig(row)">配置</el-button>
        </template>
      </el-table-column>
    </el-table>

    <InterfaceDialog v-model:visible="dialogVisible" :iface="editing" :capabilities="props.capabilities" @saved="load" />
  </section>
</template>

<style scoped>
.iface-mac {
  font-size: 12px;
  color: var(--color-text-dim);
}
</style>
