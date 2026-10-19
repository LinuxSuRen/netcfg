<script setup>
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { setIP, setDNS } from '../api'

const props = defineProps({
  visible: { type: Boolean, default: false },
  iface: { type: Object, default: null },
  capabilities: { type: Object, default: null }
})

const emit = defineEmits(['update:visible', 'saved'])

const formRef = ref(null)
const saving = ref(false)

const form = reactive({
  ipMode: 'dhcp',
  ip: '',
  netmask: '',
  gateway: '',
  dnsMode: 'auto',
  servers: []
})

// 能力未知时按可用处理，由后端返回明确的错误码
const readOnlyIP = computed(() => !!props.capabilities && props.capabilities.set_ip === false)
const readOnlyDNS = computed(() => !!props.capabilities && props.capabilities.set_dns === false)
const allReadOnly = computed(() => readOnlyIP.value && readOnlyDNS.value)

function isIPv4(value) {
  const parts = String(value || '').split('.')
  return parts.length === 4 && parts.every(p => /^\d+$/.test(p) && +p >= 0 && +p <= 255)
}

function validateIP(rule, value, callback) {
  if (!value || !value.trim()) return callback(new Error('请输入 IP 地址'))
  if (!isIPv4(value.trim())) return callback(new Error('IP 地址格式不正确'))
  callback()
}

function validateNetmask(rule, value, callback) {
  if (!value || !value.trim()) return callback(new Error('请输入子网掩码'))
  if (!isIPv4(value.trim())) return callback(new Error('子网掩码格式不正确'))
  callback()
}

function validateGateway(rule, value, callback) {
  if (!value || !value.trim()) return callback()
  if (!isIPv4(value.trim())) return callback(new Error('网关地址格式不正确'))
  callback()
}

const rules = {
  ip: [{ validator: validateIP, trigger: 'blur' }],
  netmask: [{ validator: validateNetmask, trigger: 'blur' }],
  gateway: [{ validator: validateGateway, trigger: 'blur' }]
}

watch(
  () => props.visible,
  visible => {
    if (!visible) return
    const iface = props.iface || {}
    form.ipMode = iface.dhcp ? 'dhcp' : 'static'
    const v4 = (Array.isArray(iface.ips) ? iface.ips : []).find(ip => ip.family === 'ipv4')
    form.ip = v4 ? v4.address : ''
    form.netmask = v4 && v4.netmask ? v4.netmask : ''
    form.gateway = iface.gateway || ''
    const dns = Array.isArray(iface.dns) ? iface.dns : []
    form.dnsMode = dns.length ? 'manual' : 'auto'
    form.servers = [...dns]
    nextTick(() => formRef.value && formRef.value.clearValidate())
  }
)

async function handleSave() {
  if (form.ipMode === 'static') {
    try {
      await formRef.value.validate()
    } catch (e) {
      return
    }
  }
  const iface = props.iface
  if (!iface || !iface.name) return

  saving.value = true
  try {
    // 依次提交 IP 与 DNS，均按对话框当前值提交
    if (!readOnlyIP.value) {
      await setIP(iface.name, {
        dhcp: form.ipMode === 'dhcp',
        ip: form.ip.trim(),
        netmask: form.netmask.trim(),
        gateway: form.gateway.trim()
      })
    }
    if (!readOnlyDNS.value) {
      await setDNS(iface.name, {
        auto: form.dnsMode === 'auto',
        servers: form.dnsMode === 'manual' ? form.servers.map(s => String(s).trim()).filter(Boolean) : []
      })
    }
    ElMessage.success('已保存')
    emit('saved')
    emit('update:visible', false)
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    saving.value = false
  }
}

function handleClose() {
  emit('update:visible', false)
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="`配置 ${iface ? iface.name : ''}`"
    width="560px"
    @update:model-value="handleClose"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <div class="dialog-section">
        <div class="dialog-section__head">
          <span class="dialog-section__title">IP 设置</span>
          <span v-if="readOnlyIP" class="dialog-section__hint">当前系统不支持修改网卡 IP</span>
        </div>
        <el-form-item label="IP 模式">
          <el-radio-group v-model="form.ipMode" :disabled="readOnlyIP">
            <el-radio value="dhcp">DHCP</el-radio>
            <el-radio value="static">静态</el-radio>
          </el-radio-group>
        </el-form-item>
        <template v-if="form.ipMode === 'static'">
          <el-form-item label="IP 地址" prop="ip">
            <el-input v-model="form.ip" :disabled="readOnlyIP" placeholder="如 192.168.1.10" />
          </el-form-item>
          <el-form-item label="子网掩码" prop="netmask">
            <el-input v-model="form.netmask" :disabled="readOnlyIP" placeholder="255.255.255.0" />
          </el-form-item>
          <el-form-item label="网关" prop="gateway">
            <el-input v-model="form.gateway" :disabled="readOnlyIP" placeholder="如 192.168.1.1" />
          </el-form-item>
        </template>
      </div>

      <div class="dialog-section">
        <div class="dialog-section__head">
          <span class="dialog-section__title">DNS 设置</span>
          <span v-if="readOnlyDNS" class="dialog-section__hint">当前系统不支持修改 DNS</span>
        </div>
        <el-form-item label="DNS 模式">
          <el-radio-group v-model="form.dnsMode" :disabled="readOnlyDNS">
            <el-radio value="auto">自动</el-radio>
            <el-radio value="manual">手动</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.dnsMode === 'manual'" label="DNS 服务器">
          <el-select
            v-model="form.servers"
            :disabled="readOnlyDNS"
            class="dns-servers"
            multiple
            filterable
            allow-create
            default-first-option
            :reserve-keyword="false"
            placeholder="输入地址后回车添加"
          >
            <el-option v-for="server in form.servers" :key="server" :label="server" :value="server" />
          </el-select>
        </el-form-item>
      </div>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">取消</el-button>
      <el-button type="primary" :loading="saving" :disabled="allReadOnly" @click="handleSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-section {
  padding-top: var(--space-2);
}

.dialog-section + .dialog-section {
  margin-top: var(--space-4);
  border-top: 1px solid var(--color-border-subtle);
  padding-top: var(--space-4);
}

.dialog-section__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  margin-bottom: var(--space-2);
}

.dialog-section__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.dialog-section__hint {
  font-size: 13px;
  color: var(--color-warning);
}

.dns-servers {
  width: 100%;
}
</style>
