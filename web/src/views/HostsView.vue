<script setup>
import { nextTick, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { listHosts, addHost, removeHost } from '../api'

const entries = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const formRef = ref(null)

const form = reactive({
  ip: '',
  hostname: ''
})

function isIPv4(value) {
  const parts = String(value || '').split('.')
  return parts.length === 4 && parts.every(p => /^\d+$/.test(p) && +p >= 0 && +p <= 255)
}

function validateIP(rule, value, callback) {
  if (!value || !value.trim()) return callback(new Error('请输入 IP 地址'))
  if (!isIPv4(value.trim())) return callback(new Error('IP 地址格式不正确'))
  callback()
}

function validateHostname(rule, value, callback) {
  if (!value || !value.trim()) return callback(new Error('请输入域名'))
  if (!/^[a-zA-Z0-9._-]+$/.test(value.trim())) return callback(new Error('域名格式不正确'))
  callback()
}

const rules = {
  ip: [{ validator: validateIP, trigger: 'blur' }],
  hostname: [{ validator: validateHostname, trigger: 'blur' }]
}

async function load() {
  loading.value = true
  try {
    const data = await listHosts()
    entries.value = Array.isArray(data) ? data : []
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

function openAdd() {
  form.ip = ''
  form.hostname = ''
  dialogVisible.value = true
  nextTick(() => formRef.value && formRef.value.clearValidate())
}

async function handleAdd() {
  try {
    await formRef.value.validate()
  } catch (e) {
    return
  }
  saving.value = true
  try {
    await addHost({ ip: form.ip.trim(), hostname: form.hostname.trim() })
    ElMessage.success('已添加')
    dialogVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    saving.value = false
  }
}

async function handleRemove(entry, hostname) {
  try {
    await ElMessageBox.confirm(`确定删除 ${hostname} 吗？`, '删除域名', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch (e) {
    return
  }
  try {
    await removeHost(hostname)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    ElMessage.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <section class="block">
    <div class="block__header">
      <div>
        <h2 class="block__title">域名解析</h2>
        <p class="block__desc">维护本机 hosts 记录，netcfg 管理的条目可在此删除</p>
      </div>
      <div class="block__actions">
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openAdd">添加</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="entries" border>
      <el-table-column label="IP 地址" min-width="140">
        <template #default="{ row }">
          <span class="mono">{{ row.ip }}</span>
        </template>
      </el-table-column>
      <el-table-column label="域名" min-width="200">
        <template #default="{ row }">
          <span>{{ (row.hostnames || []).join(' ') }}</span>
        </template>
      </el-table-column>
      <el-table-column label="来源" width="120">
        <template #default="{ row }">
          <el-tag :type="row.managed ? 'success' : 'info'" size="small">
            {{ row.managed ? 'netcfg 管理' : '系统' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" min-width="160" fixed="right">
        <template #default="{ row }">
          <template v-if="row.managed">
            <div v-for="hostname in row.hostnames || []" :key="hostname" class="host-remove">
              <el-button link type="danger" size="small" @click="handleRemove(row, hostname)">
                删除 {{ hostname }}
              </el-button>
            </div>
          </template>
          <span v-else class="text-dim">—</span>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      :model-value="dialogVisible"
      title="添加域名解析"
      width="440px"
      @update:model-value="value => (dialogVisible = value)"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="70px">
        <el-form-item label="IP 地址" prop="ip">
          <el-input v-model="form.ip" placeholder="如 192.168.1.10" />
        </el-form-item>
        <el-form-item label="域名" prop="hostname">
          <el-input v-model="form.hostname" placeholder="如 example.local" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleAdd">添加</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.host-remove {
  line-height: 1.8;
}
</style>
