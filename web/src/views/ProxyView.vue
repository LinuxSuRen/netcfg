<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getProxy, setProxy } from '../api'

const props = defineProps({
  capabilities: { type: Object, default: null }
})

const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)

const form = reactive({
  mode: 'none',
  http_host: '',
  http_port: null,
  https_host: '',
  https_port: null,
  bypass: ''
})

const proxySupported = computed(() => !props.capabilities || props.capabilities.proxy !== false)
const manual = computed(() => form.mode === 'manual')

const rules = {
  http_host: [{ required: true, message: '请输入 HTTP 服务器地址', trigger: 'blur' }],
  http_port: [
    {
      required: true,
      validator(rule, value, callback) {
        if (value === null || value === undefined || value === '') return callback(new Error('请输入 HTTP 端口'))
        if (!Number.isInteger(value) || value < 1 || value > 65535) return callback(new Error('端口范围 1 到 65535'))
        callback()
      },
      trigger: 'blur'
    }
  ],
  https_port: [
    {
      validator(rule, value, callback) {
        if (value === null || value === undefined || value === '') return callback()
        if (!Number.isInteger(value) || value < 1 || value > 65535) return callback(new Error('端口范围 1 到 65535'))
        callback()
      },
      trigger: 'blur'
    }
  ]
}

async function load() {
  loading.value = true
  try {
    const cfg = await getProxy()
    if (cfg) {
      form.mode = cfg.mode === 'manual' ? 'manual' : 'none'
      form.http_host = cfg.http_host || ''
      form.http_port = cfg.http_port || null
      form.https_host = cfg.https_host || ''
      form.https_port = cfg.https_port || null
      form.bypass = Array.isArray(cfg.bypass) ? cfg.bypass.join(', ') : ''
    }
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  if (manual.value) {
    try {
      await formRef.value.validate()
    } catch (e) {
      return
    }
  }
  saving.value = true
  try {
    await setProxy({
      mode: form.mode,
      http_host: form.http_host.trim(),
      http_port: form.http_port === null ? 0 : Number(form.http_port),
      https_host: form.https_host.trim(),
      https_port: form.https_port === null ? 0 : Number(form.https_port),
      bypass: form.bypass
        .split(',')
        .map(item => item.trim())
        .filter(Boolean)
    })
    ElMessage.success('已保存')
    await load()
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="block">
    <div class="block__header">
      <div>
        <h2 class="block__title">系统代理</h2>
        <p class="block__desc">配置本机 HTTP 与 HTTPS 代理及绕过列表</p>
      </div>
      <div class="block__actions">
        <el-button type="primary" :loading="saving" :disabled="!proxySupported" @click="handleSave">保存</el-button>
      </div>
    </div>

    <el-alert
      v-if="!proxySupported"
      type="warning"
      :closable="false"
      title="当前系统不支持修改系统代理"
      class="proxy-alert"
    />

    <el-form
      ref="formRef"
      v-loading="loading"
      :model="form"
      :rules="rules"
      :disabled="!proxySupported"
      label-width="110px"
      class="proxy-form"
    >
      <el-form-item label="代理模式">
        <el-radio-group v-model="form.mode">
          <el-radio value="none">不使用代理</el-radio>
          <el-radio value="manual">手动配置</el-radio>
        </el-radio-group>
      </el-form-item>

      <template v-if="manual">
        <el-form-item label="HTTP 代理">
          <div class="proxy-pair">
            <el-input v-model="form.http_host" placeholder="代理服务器地址" class="proxy-pair__host" />
            <el-input-number
              v-model="form.http_port"
              :min="1"
              :max="65535"
              :controls="false"
              placeholder="端口"
              class="proxy-pair__port"
            />
          </div>
        </el-form-item>
        <el-form-item label="HTTPS 代理">
          <div class="proxy-pair">
            <el-input v-model="form.https_host" placeholder="留空时与 HTTP 代理一致" class="proxy-pair__host" />
            <el-input-number
              v-model="form.https_port"
              :min="1"
              :max="65535"
              :controls="false"
              placeholder="端口"
              class="proxy-pair__port"
            />
          </div>
        </el-form-item>
        <el-form-item label="绕过列表">
          <el-input v-model="form.bypass" type="textarea" :rows="2" placeholder="多个地址用逗号分隔" />
        </el-form-item>
      </template>
    </el-form>
  </section>
</template>

<style scoped>
.proxy-alert {
  margin-bottom: var(--space-3);
}

.proxy-form {
  max-width: 560px;
}

.proxy-pair {
  display: flex;
  gap: var(--space-2);
  width: 100%;
}

.proxy-pair__host {
  flex: 1;
}

.proxy-pair__port {
  width: 110px;
  flex-shrink: 0;
}
</style>
