<script lang="ts" setup>
import {computed, h, onMounted, onUnmounted, reactive, ref, watch} from 'vue'
import {message, Modal} from 'ant-design-vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import {
  CloudSyncOutlined,
  FolderOpenOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  StopOutlined,
  TranslationOutlined,
} from '@ant-design/icons-vue'
import {
  ChooseFolder,
  FetchModels,
  LoadLocalization,
  SaveRow,
  SetPaused,
  StartTranslation,
  StopTranslation,
  TranslateRow,
} from '../wailsjs/go/main/App'
import {main} from '../wailsjs/go/models'
import {EventsOn, OnFileDrop, OnFileDropOff} from '../wailsjs/runtime/runtime'

type Row = main.LocalizationRow
type TableFilter = 'all' | 'failed' | 'pending' | 'translated' | 'skipped'

const API_CONFIG_STORAGE_KEY = 'seven-days-mod-localizer-api-config'

const path = ref('')
const documentInfo = ref<main.LocalizationDocument | null>(null)
const rows = ref<Row[]>([])
const loadingFile = ref(false)
const fetchingModels = ref(false)
const translating = ref(false)
const paused = ref(false)
const statusText = ref('等待加载 Localization 文件')
const models = ref<main.ModelInfo[]>([])
const selectedRowKeys = ref<number[]>([])
const tablePageSize = ref(50)
const tableFilter = ref<TableFilter>('all')
const failedRowIndexes = ref<number[]>([])

const apiForm = reactive({
  baseUrl: '',
  apiKey: '',
  model: '',
  batchSize: 15,
  concurrency: 3,
  overwrite: false,
})

const progress = reactive({
  total: 0,
  completed: 0,
  failed: 0,
  status: 'idle',
})

const editor = reactive({
  open: false,
  saving: false,
  translating: false,
  row: null as Row | null,
  value: '',
})

const columns = [
  {title: '行号', dataIndex: 'rowNumber', key: 'rowNumber', width: 82, fixed: 'left'},
  {title: '英文原文', dataIndex: 'english', key: 'english', width: 520},
  {title: '简体中文译文', dataIndex: 'schinese', key: 'schinese', width: 520},
  {title: '状态', key: 'status', width: 120, fixed: 'right'},
]

const pendingCount = computed(() => rows.value.filter((row) => row.translatable && !row.schinese?.trim()).length)
const translatedCount = computed(() => rows.value.filter((row) => row.translatable && row.schinese?.trim()).length)
const translatableCount = computed(() => rows.value.filter((row) => row.translatable).length)
const failedRowSet = computed(() => new Set(failedRowIndexes.value))
const failedRowsCount = computed(() => failedRowIndexes.value.length)
const filteredRows = computed(() => {
  const failed = failedRowSet.value
  const sortedRows = [...rows.value].sort((a, b) => a.rowNumber - b.rowNumber)

  switch (tableFilter.value) {
    case 'failed':
      return sortedRows.filter((row) => failed.has(row.index))
    case 'pending':
      return sortedRows.filter((row) => row.translatable && !row.schinese?.trim())
    case 'translated':
      return sortedRows.filter((row) => row.translatable && !!row.schinese?.trim())
    case 'skipped':
      return sortedRows.filter((row) => !row.translatable)
    default:
      return sortedRows
  }
})
const progressPercent = computed(() => {
  if (!progress.total) return 0
  return Math.round(((progress.completed + progress.failed) / progress.total) * 100)
})
const progressDetail = computed(() => {
  const handled = progress.completed + progress.failed
  let state = statusText.value

  if (progress.status === 'running') {
    if (paused.value) {
      state = '任务已暂停，正在等待继续'
    } else if (progress.total > 0 && handled === 0) {
      state = '已提交翻译任务，正在等待第一批结果'
    } else {
      state = '正在翻译并实时保存结果'
    }
  } else if (progress.status === 'canceled') {
    state = '任务已终止'
  } else if (progress.status === 'done' && !state) {
    state = '任务已完成'
  }

  if (!progress.total) {
    return state
  }
  return `${state}。本次进度：已处理 ${handled}/${progress.total} 条（完成 ${progress.completed} 条，失败 ${progress.failed} 条）`
})

const rowSelection = computed(() => ({
  selectedRowKeys: selectedRowKeys.value,
  onChange: (keys: number[]) => {
    selectedRowKeys.value = keys
  },
}))

const tablePagination = computed(() => ({
  pageSize: tablePageSize.value,
  showSizeChanger: false,
  showTotal: (total: number) => `共 ${total} 条`,
}))

let offProgress: (() => void) | undefined
let offError: (() => void) | undefined
let offState: (() => void) | undefined
let fileBusyModalOpen = false

function preventDefaultFileDrop(event: DragEvent) {
  event.preventDefault()
}

onMounted(() => {
  loadSavedApiConfig()
  window.addEventListener('dragover', preventDefaultFileDrop)
  window.addEventListener('drop', preventDefaultFileDrop)
  offProgress = EventsOn('translation:progress', (payload: any) => {
    progress.total = payload.total || 0
    progress.completed = payload.completed || 0
    progress.failed = payload.failed || 0
    progress.status = payload.status || 'running'
    if (Array.isArray(payload.rows) && payload.rows.length > 0) {
      mergeUpdatedRows(payload.rows)
      removeFailedRows(payload.rows)
    }
    if (Array.isArray(payload.failedRows) && payload.failedRows.length > 0) {
      markFailedRows(payload.failedRows)
    }
    if (payload.status === 'done' || payload.status === 'canceled') {
      translating.value = false
      paused.value = false
    }
  })
  offError = EventsOn('translation:error', (err: string) => {
    showAppError(err, 'warning')
  })
  offState = EventsOn('translation:state', (state: string) => {
    paused.value = state === 'paused'
  })
  OnFileDrop((_x: number, _y: number, paths: string[]) => {
    if (paths?.length) {
      path.value = paths[0]
      void loadLocalization()
    }
  }, false)
})

onUnmounted(() => {
  window.removeEventListener('dragover', preventDefaultFileDrop)
  window.removeEventListener('drop', preventDefaultFileDrop)
  offProgress?.()
  offError?.()
  offState?.()
  OnFileDropOff()
})

watch(
  apiForm,
  () => {
    saveApiConfig()
  },
  {deep: true},
)

function loadSavedApiConfig() {
  try {
    const raw = localStorage.getItem(API_CONFIG_STORAGE_KEY)
    if (!raw) return
    const saved = JSON.parse(raw)
    if (typeof saved.baseUrl === 'string') apiForm.baseUrl = saved.baseUrl
    if (typeof saved.apiKey === 'string') apiForm.apiKey = saved.apiKey
    if (typeof saved.model === 'string') apiForm.model = saved.model
    if (typeof saved.batchSize === 'number') apiForm.batchSize = saved.batchSize
    if (typeof saved.concurrency === 'number') apiForm.concurrency = saved.concurrency
    if (typeof saved.overwrite === 'boolean') apiForm.overwrite = saved.overwrite
  } catch {
    localStorage.removeItem(API_CONFIG_STORAGE_KEY)
  }
}

function saveApiConfig() {
  try {
    localStorage.setItem(
      API_CONFIG_STORAGE_KEY,
      JSON.stringify({
        baseUrl: apiForm.baseUrl,
        apiKey: apiForm.apiKey,
        model: apiForm.model,
        batchSize: apiForm.batchSize,
        concurrency: apiForm.concurrency,
        overwrite: apiForm.overwrite,
      }),
    )
  } catch {
    // 本地存储不可用时不影响当前翻译流程。
  }
}

function getErrorMessage(err: unknown) {
  if (typeof err === 'string') return err
  if (err && typeof err === 'object' && 'message' in err) {
    return String((err as {message?: unknown}).message)
  }
  return String(err)
}

function isFileBusyErrorMessage(text: string) {
  const lower = text.toLowerCase()
  return (
    lower.includes('localization 文件被其他程序占用') ||
    lower.includes('localization 文件被占用') ||
    lower.includes('being used by another process') ||
    lower.includes('process cannot access the file') ||
    lower.includes('sharing violation') ||
    lower.includes('lock violation') ||
    lower.includes('文件正由另一进程使用') ||
    lower.includes('另一个程序正在使用此文件') ||
    lower.includes('另一个进程正在使用此文件') ||
    lower.includes('该文件正由另一个进程使用')
  )
}

function showFileBusyModal(text: string) {
  if (fileBusyModalOpen) return
  fileBusyModalOpen = true
  Modal.error({
    title: 'Localization 文件被占用，无法保存',
    content: h('div', {style: {whiteSpace: 'pre-line'}}, text),
    okText: '我知道了',
    onOk: () => {
      fileBusyModalOpen = false
    },
    afterClose: () => {
      fileBusyModalOpen = false
    },
  })
}

function showAppError(err: unknown, level: 'error' | 'warning' = 'error') {
  const text = getErrorMessage(err)
  if (isFileBusyErrorMessage(text)) {
    showFileBusyModal(text)
    return
  }
  if (level === 'warning') {
    message.warning(text)
    return
  }
  message.error(text)
}

async function chooseFolder() {
  try {
    const selected = await ChooseFolder()
    if (!selected) return
    path.value = selected
    await loadLocalization()
  } catch (err: any) {
    showAppError(err)
  }
}

async function loadLocalization() {
  if (!path.value.trim()) {
    message.warning('请先选择单个 Mod 目录，或输入 Localization 文件路径')
    return
  }
  loadingFile.value = true
  try {
    const doc = await LoadLocalization(path.value.trim())
    documentInfo.value = doc
    rows.value = doc.rows || []
    selectedRowKeys.value = []
    failedRowIndexes.value = []
    tableFilter.value = 'all'
    progress.total = 0
    progress.completed = 0
    progress.failed = 0
    progress.status = 'idle'
    statusText.value = `已加载：${doc.path}`
    message.success(`加载成功：${doc.totalRows} 行，可翻译 ${translatableCount.value} 行`)
  } catch (err: any) {
    showAppError(err)
  } finally {
    loadingFile.value = false
  }
}

async function fetchModels() {
  if (!apiForm.baseUrl.trim()) {
    message.warning('请先填写接口地址 Base URL')
    return
  }
  fetchingModels.value = true
  try {
    models.value = await FetchModels(apiForm.baseUrl.trim(), apiForm.apiKey.trim())
    if (models.value.length > 0 && !apiForm.model) {
      apiForm.model = models.value[0].id
    }
    message.success(`获取到 ${models.value.length} 个模型`)
  } catch (err: any) {
    showAppError(err)
  } finally {
    fetchingModels.value = false
  }
}

async function startTranslation() {
  if (!documentInfo.value) {
    message.warning('请先加载 Localization 文件')
    return
  }
  if (!apiForm.baseUrl.trim() || !apiForm.apiKey.trim() || !apiForm.model.trim()) {
    message.warning('请填写接口地址、接口密钥并选择模型')
    return
  }
  translating.value = true
  paused.value = false
  statusText.value = '准备提交翻译任务'
  progress.total = 0
  progress.completed = 0
  progress.failed = 0
  progress.status = 'running'
  failedRowIndexes.value = []

  try {
    const summary = await StartTranslation({
      baseUrl: apiForm.baseUrl.trim(),
      apiKey: apiForm.apiKey.trim(),
      model: apiForm.model.trim(),
      batchSize: apiForm.batchSize,
      concurrency: apiForm.concurrency,
      overwrite: apiForm.overwrite,
    } as main.TranslateOptions)
    statusText.value = summary.message || '翻译任务结束'
    if (isFileBusyErrorMessage(statusText.value)) {
      showAppError(statusText.value)
    } else if (summary.failed > 0) {
      message.warning(statusText.value)
    } else {
      message.success(statusText.value)
    }
  } catch (err: any) {
    showAppError(err)
  } finally {
    translating.value = false
    paused.value = false
  }
}

async function togglePause() {
  paused.value = !paused.value
  await SetPaused(paused.value)
  statusText.value = paused.value ? '任务已暂停' : '任务继续运行'
}

async function stopTranslation() {
  await StopTranslation()
  statusText.value = '正在终止任务...'
}

function openEditor(row: Row) {
  editor.row = row
  editor.value = row.schinese || ''
  editor.open = true
}

async function translateEditorRow() {
  if (!editor.row) return
  if (!editor.row.english?.trim()) {
    message.warning('当前行没有英文原文，无法翻译')
    return
  }
  if (!apiForm.baseUrl.trim() || !apiForm.apiKey.trim() || !apiForm.model.trim()) {
    message.warning('请先填写接口地址、接口密钥并选择模型')
    return
  }

  const messageKey = 'single-row-translate'
  editor.translating = true
  message.loading({content: '正在翻译本条，最多等待 10 秒...', key: messageKey, duration: 0})
  try {
    const translated = await TranslateRow(editor.row.index, {
      baseUrl: apiForm.baseUrl.trim(),
      apiKey: apiForm.apiKey.trim(),
      model: apiForm.model.trim(),
      batchSize: 1,
      concurrency: 1,
      overwrite: true,
    } as main.TranslateOptions)
    editor.value = translated
    message.success({content: '已翻译本条，请确认后保存', key: messageKey, duration: 2})
  } catch (err: any) {
    message.error({content: err?.message || String(err), key: messageKey, duration: 3})
  } finally {
    editor.translating = false
  }
}

async function saveEditor() {
  if (!editor.row) return
  editor.saving = true
  try {
    const updated = await SaveRow(editor.row.index, editor.value)
    mergeUpdatedRows([updated])
    editor.open = false
    message.success('已保存到 Localization 文件')
  } catch (err: any) {
    showAppError(err)
  } finally {
    editor.saving = false
  }
}

function mergeUpdatedRows(updatedRows: Row[]) {
  const map = new Map(rows.value.map((row) => [row.index, row]))
  for (const row of updatedRows) {
    map.set(row.index, row)
  }
  rows.value = rows.value.map((row) => map.get(row.index) || row)
}

function markFailedRows(failedRows: Row[]) {
  const failed = new Set(failedRowIndexes.value)
  for (const row of failedRows) {
    failed.add(row.index)
  }
  failedRowIndexes.value = [...failed].sort((a, b) => a - b)
}

function removeFailedRows(updatedRows: Row[]) {
  if (failedRowIndexes.value.length === 0) return
  const updated = new Set(updatedRows.map((row) => row.index))
  failedRowIndexes.value = failedRowIndexes.value.filter((index) => !updated.has(index))
}

function tagColor(row: Row) {
  if (failedRowSet.value.has(row.index)) return 'red'
  if (!row.translatable) return 'default'
  if (row.schinese?.trim()) return 'green'
  return 'orange'
}

function tagText(row: Row) {
  if (failedRowSet.value.has(row.index)) return '翻译失败'
  if (!row.translatable) {
    if (!row.english?.trim()) return '无英文原文'
    if (row.noTranslate?.trim()) return `模组要求不翻译：${row.noTranslate}`
    return '不可翻译'
  }
  if (row.schinese?.trim()) return '已翻译'
  return '待翻译'
}
</script>

<template>
  <a-config-provider
    :locale="zhCN"
    :theme="{
      token: {
        colorPrimary: '#1677ff',
        borderRadius: 8,
      },
    }"
  >
    <div class="app-shell">
      <a-row :gutter="[16, 16]">
        <a-col :span="24">
          <a-card class="drop-card" :bordered="false">
            <div class="path-row">
              <a-input-search
                v-model:value="path"
                size="large"
                enter-button="加载"
                placeholder="拖入单个 Mod 目录，或输入 Localization.csv / Localization.txt 路径"
                :loading="loadingFile"
                @search="loadLocalization"
              />
              <a-button size="large" :loading="loadingFile" @click="loadLocalization">
                <template #icon><ReloadOutlined /></template>
                重新加载
              </a-button>
              <a-button type="primary" size="large" :loading="loadingFile" @click="chooseFolder">
                <template #icon><FolderOpenOutlined /></template>
                选择 Mod 目录
              </a-button>
            </div>
            <div class="drop-hint">将需要汉化的单个 Mod 目录拖动到此处，不要选择整个 Mods 汇总目录</div>
          </a-card>
        </a-col>

        <a-col :xs="24" :xl="10">
          <a-card title="接口与模型" class="compact-card" :bordered="false">
            <a-form layout="vertical">
              <a-row :gutter="12">
                <a-col :xs="24" :md="12">
                  <a-form-item label="接口地址 Base URL">
                    <a-input v-model:value="apiForm.baseUrl" placeholder="例如：https://api.example.com 或 http://127.0.0.1:3000" />
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :md="12">
                  <a-form-item label="接口密钥 API Key">
                    <a-input v-model:value="apiForm.apiKey" placeholder="填写站点密钥，明文显示并保存在本机" />
                  </a-form-item>
                </a-col>
              </a-row>
              <div class="form-help">
                兼容 OpenAI 接口协议：程序会自动请求 /v1/models 和 /v1/chat/completions。接口地址、密钥、模型和参数会自动保存在本机，下次打开自动填充。
              </div>
              <a-form-item label="模型">
                <a-input-group compact>
                  <a-select
                    v-model:value="apiForm.model"
                    show-search
                    placeholder="先获取模型或手动输入"
                    style="width: calc(100% - 104px)"
                    :options="models.map((model) => ({ label: model.id, value: model.id }))"
                  />
                  <a-button :loading="fetchingModels" style="width: 104px" @click="fetchModels">获取模型</a-button>
                </a-input-group>
              </a-form-item>
              <div class="inline-options">
                <div>
                  <span class="option-label">单次翻译数量</span>
                  <a-input-number v-model:value="apiForm.batchSize" :min="1" :max="50" />
                </div>
                <div>
                  <span class="option-label">同时翻译线程</span>
                  <a-input-number v-model:value="apiForm.concurrency" :min="1" :max="20" />
                </div>
                <a-checkbox v-model:checked="apiForm.overwrite">覆盖已有翻译内容</a-checkbox>
              </div>
              <div class="form-help">建议：单次 10-20 条、线程 3-20 个；每批成功后会立即写回文件，方便中断后续跑。</div>
            </a-form>
          </a-card>
        </a-col>

        <a-col :xs="24" :xl="14">
          <a-card title="任务进度" class="compact-card" :bordered="false">
            <div class="metric-row">
              <a-statistic title="总行数" :value="documentInfo?.totalRows || 0" />
              <a-statistic title="可翻译" :value="translatableCount" />
              <a-statistic title="已翻译" :value="translatedCount" />
              <a-statistic title="待翻译" :value="pendingCount" />
              <a-statistic title="失败" :value="failedRowsCount" />
            </div>
            <a-progress
              :percent="progressPercent"
              :status="progress.failed > 0 ? 'exception' : progress.status === 'done' ? 'success' : 'active'"
            />
            <div class="progress-detail">
              {{ progressDetail }}
            </div>

            <a-space wrap>
              <a-button type="primary" :disabled="translating" @click="startTranslation">
                <template #icon><CloudSyncOutlined /></template>
                开始汉化
              </a-button>
              <a-button :disabled="!translating" @click="togglePause">
                <template #icon>
                  <PlayCircleOutlined v-if="paused" />
                  <PauseCircleOutlined v-else />
                </template>
                {{ paused ? '继续' : '暂停' }}
              </a-button>
              <a-button danger :disabled="!translating" @click="stopTranslation">
                <template #icon><StopOutlined /></template>
                终止
              </a-button>
            </a-space>
          </a-card>
        </a-col>

        <a-col :span="24">
          <a-card :bordered="false">
            <template #title>
              <div class="table-title">
                <span>游戏文本原文与译文</span>
              </div>
            </template>
            <template #extra>
              <a-space wrap>
                <span class="muted">双击“译文”单元格可手动编辑并保存</span>
                <a-select v-model:value="tableFilter" style="width: 150px">
                  <a-select-option value="all">原行号顺序</a-select-option>
                  <a-select-option value="failed">仅失败条目</a-select-option>
                  <a-select-option value="pending">仅待翻译</a-select-option>
                  <a-select-option value="translated">仅已翻译</a-select-option>
                  <a-select-option value="skipped">仅跳过</a-select-option>
                </a-select>
                <a-select v-model:value="tablePageSize" style="width: 110px">
                  <a-select-option :value="20">20 / 页</a-select-option>
                  <a-select-option :value="50">50 / 页</a-select-option>
                  <a-select-option :value="100">100 / 页</a-select-option>
                </a-select>
              </a-space>
            </template>

            <a-table
              row-key="index"
              size="middle"
              :columns="columns"
              :data-source="filteredRows"
              :loading="loadingFile"
              :row-selection="rowSelection"
              :pagination="tablePagination"
              :scroll="{ x: 1240, y: 'calc(100vh - 360px)' }"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'english'">
                  <div class="cell-text source-text">{{ record.english || '-' }}</div>
                </template>
                <template v-else-if="column.key === 'schinese'">
                  <div class="cell-text target-text" @dblclick="openEditor(record)">
                    {{ record.schinese || '双击填写/编辑翻译' }}
                  </div>
                </template>
                <template v-else-if="column.key === 'status'">
                  <a-tag :color="tagColor(record)">{{ tagText(record) }}</a-tag>
                </template>
              </template>
            </a-table>
          </a-card>
        </a-col>
      </a-row>

      <a-modal
        v-model:open="editor.open"
        title="编辑简体中文翻译"
        width="760px"
        :confirm-loading="editor.saving"
        :ok-button-props="{ disabled: editor.translating }"
        @ok="saveEditor"
      >
        <a-descriptions v-if="editor.row" size="small" bordered :column="1">
          <a-descriptions-item label="原文">{{ editor.row.english }}</a-descriptions-item>
        </a-descriptions>
        <div class="editor-actions">
          <a-button
            type="primary"
            ghost
            :loading="editor.translating"
            :disabled="editor.saving || !editor.row?.english?.trim()"
            @click="translateEditorRow"
          >
            <template #icon><TranslationOutlined /></template>
            翻译本条
          </a-button>
          <span class="muted">只翻译当前原文，10 秒无响应自动超时；结果会填入下方文本框，确认后再保存。</span>
        </div>
        <a-textarea v-model:value="editor.value" class="editor-textarea" :rows="8" />
        <template #okText>
          <SaveOutlined />
          保存
        </template>
      </a-modal>
    </div>
  </a-config-provider>
</template>
