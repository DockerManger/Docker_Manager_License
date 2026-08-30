<template>
  <div class="max-w-3xl space-y-5 fade-up">
    <PageHeader :title="$t('settings.title')" :description="$t('settings.description')" />

    <Tabs v-model="tab" class="w-full">
      <TabsList class="w-fit">
        <TabsTrigger value="account">
          <ShieldCheck class="size-3.5" /> {{ $t('settings.tabAccount') }}
        </TabsTrigger>
        <TabsTrigger value="versions">
          <Tags class="size-3.5" /> {{ $t('settings.tabVersions') }}
        </TabsTrigger>
        <TabsTrigger value="keys">
          <KeyRound class="size-3.5" /> {{ $t('settings.tabKeys') }}
        </TabsTrigger>
      </TabsList>

      <!-- ============ 账户安全 ============ -->
      <TabsContent value="account" class="space-y-5 mt-4">
        <Card class="p-5">
          <h3 class="text-[14px] font-semibold mb-1.5">{{ $t('settings.changePassword') }}</h3>
          <p class="text-[12px] text-muted mb-4">{{ $t('settings.changePasswordDesc') }}</p>
          <form class="space-y-3.5 max-w-sm" @submit.prevent="changePassword">
            <div>
              <Label>{{ $t('settings.currentPwd') }}</Label>
              <Input v-model="pw.old" type="password" autocomplete="current-password" required />
            </div>
            <div>
              <Label>{{ $t('settings.newPwd') }} <span class="text-muted font-normal">{{ $t('settings.newPwdHint') }}</span></Label>
              <Input v-model="pw.new1" type="password" autocomplete="new-password" required />
            </div>
            <div>
              <Label>{{ $t('settings.confirmPwd') }}</Label>
              <Input v-model="pw.new2" type="password" autocomplete="new-password" required />
            </div>
            <p v-if="pwErr" class="text-danger text-[12.5px] flex items-center gap-1.5">
              <AlertCircle class="size-3.5 shrink-0" /> {{ pwErr }}
            </p>
            <Button type="submit" variant="primary" :disabled="pwBusy">
              <Loader2 v-if="pwBusy" class="size-3.5 animate-spin" />
              {{ $t('settings.changeBtn') }}
            </Button>
          </form>
        </Card>

        <Card class="p-5">
          <div class="flex items-center justify-between mb-1.5">
            <h3 class="text-[14px] font-semibold">{{ $t('settings.twofa') }}</h3>
            <Button v-if="totpEnabled" variant="ghost" size="sm" class="text-danger" @click="disableTOTP">
              <ShieldOff class="size-3.5" /> {{ $t('settings.disable2fa') }}
            </Button>
          </div>
          <p class="text-[12px] text-muted mb-4">{{ $t('settings.twofaDesc') }}</p>

          <div v-if="!totpSecret" class="flex justify-end">
            <Button variant="ghost" @click="setupTOTP">
              <QrCode class="size-3.5" /> {{ $t('settings.genSecret') }}
            </Button>
          </div>

          <div v-if="totpSecret" class="rounded-[0.5rem] border border-line bg-surface2/50 p-4 space-y-3">
            <div class="text-[12.5px] text-muted">
              <p>{{ $t('settings.step1') }}</p>
              <div class="font-mono text-text mt-1.5 select-all bg-bg rounded-[0.35rem] px-3 py-2 text-[12px] break-all">
                {{ totpSecret }}
              </div>
            </div>
            <div class="text-[12.5px] text-muted">{{ $t('settings.step2') }}</div>
            <div class="flex gap-2 max-w-xs">
              <Input v-model="totpCode" :placeholder="$t('settings.enterCode')" inputmode="numeric" />
              <Button variant="primary" :disabled="totpBusy" @click="confirmTOTP">
                <Loader2 v-if="totpBusy" class="size-3.5 animate-spin" />
                {{ $t('settings.confirmEnable') }}
              </Button>
            </div>
            <p v-if="totpErr" class="text-danger text-[12.5px]">{{ totpErr }}</p>
          </div>
        </Card>
      </TabsContent>

      <!-- ============ 客户端版本控制 ============ -->
      <TabsContent value="versions" class="mt-4">
        <Card class="p-5">
          <h3 class="text-[14px] font-semibold mb-1.5">{{ $t('settings.versionControl') }}</h3>
          <p class="text-[12px] text-muted mb-5 leading-relaxed whitespace-pre-line">
            {{ $t('settings.versionControlDesc') }}
          </p>

          <div class="space-y-5 max-w-md">
            <div>
              <Label>{{ $t('settings.minClientVersion') }}</Label>
              <Input v-model="settings.minimum_client_version" :placeholder="$t('settings.minVerPlaceholder')" />
              <p class="text-[11px] text-muted mt-1.5">{{ $t('settings.minClientVersionDesc') }}</p>
            </div>
            <div>
              <Label>{{ $t('settings.blockedVersions') }}</Label>
              <Textarea
                v-model="blockedText"
                rows="4"
                class="font-mono text-[12px]"
                :placeholder="$t('settings.blockedPlaceholder')"
                spellcheck="false"
              />
              <p class="text-[11px] text-muted mt-1.5">{{ $t('settings.blockedVersionsDesc') }}</p>
            </div>
            <p v-if="versionErr" class="text-danger text-[12.5px] flex items-center gap-1.5">
              <AlertCircle class="size-3.5 shrink-0" /> {{ versionErr }}
            </p>
            <Button variant="primary" :disabled="versionBusy" @click="saveSettings">
              <Loader2 v-if="versionBusy" class="size-3.5 animate-spin" />
              {{ $t('settings.saveConfig') }}
            </Button>
          </div>
        </Card>
      </TabsContent>

      <!-- ============ 签名密钥 ============ -->
      <TabsContent value="keys" class="mt-4">
        <Card class="overflow-hidden">
          <div class="px-5 py-4 border-b border-line">
            <h3 class="text-[14px] font-semibold">{{ $t('settings.signingKeys') }}</h3>
            <p class="text-[12px] text-muted mt-0.5">{{ $t('settings.signingKeysDesc') }}</p>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ $t('settings.keyId') }}</TableHead>
                <TableHead>{{ $t('settings.algorithm') }}</TableHead>
                <TableHead>{{ $t('settings.status') }}</TableHead>
                <TableHead>{{ $t('settings.createdAt') }}</TableHead>
                <TableHead>{{ $t('settings.publicKey') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="k in keys" :key="k.key_id">
                <TableCell class="font-mono text-[12px] text-brand">{{ k.key_id }}</TableCell>
                <TableCell class="font-mono text-[12px] text-muted">{{ k.algorithm }}</TableCell>
                <TableCell><StatusBadge :status="k.status === 'active' ? 'active' : 'deactivated'" /></TableCell>
                <TableCell class="text-muted text-[12px]">{{ fmtDateTimeStr(k.created_at) }}</TableCell>
                <TableCell class="font-mono text-[11px] text-muted max-w-[240px] truncate">{{ k.public_key }}</TableCell>
              </TableRow>
              <TableRow v-if="!keys.length">
                <TableCell colspan="5" class="text-center text-muted py-8">{{ $t('settings.noKeys') }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </Card>
      </TabsContent>
    </Tabs>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, KeyRound, Loader2, QrCode, ShieldCheck, ShieldOff, Tags } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import StatusBadge from '../components/StatusBadge.vue'
import PageHeader from '../components/PageHeader.vue'
import { api, type ServerSettings, type SigningKey } from '../api'
import { fmtDateTimeStr } from '../lib/format'
import { toastErr, toastOk } from '../lib/toast'

const { t } = useI18n()
const tab = ref('account')

// ---------- 修改密码 ----------
const pw = reactive({ old: '', new1: '', new2: '' })
const pwErr = ref('')
const pwBusy = ref(false)

async function changePassword() {
  pwErr.value = ''
  if (pw.new1.length < 8) {
    pwErr.value = t('settings.pwdTooShort')
    return
  }
  if (pw.new1 !== pw.new2) {
    pwErr.value = t('settings.pwdMismatch')
    return
  }
  pwBusy.value = true
  try {
    await api.post('/api/v1/admin/change-password', {
      old_password: pw.old,
      new_password: pw.new1,
    })
    toastOk(t('settings.pwdChanged'))
    setTimeout(() => {
      localStorage.clear()
      location.href = '/login'
    }, 1200)
  } catch (e: any) {
    pwErr.value = e?.message || t('settings.saveFailed')
  } finally {
    pwBusy.value = false
  }
}

// ---------- 2FA ----------
const totpSecret = ref('')
const totpCode = ref('')
const totpErr = ref('')
const totpBusy = ref(false)
const totpEnabled = ref(false)

async function setupTOTP() {
  totpErr.value = ''
  try {
    const res = await api.post<{ secret: string }>('/api/v1/admin/setup-totp')
    totpSecret.value = res.secret
  } catch (e: any) {
    totpErr.value = e?.message || t('settings.saveFailed')
  }
}

async function confirmTOTP() {
  totpBusy.value = true
  totpErr.value = ''
  try {
    await api.post('/api/v1/admin/confirm-totp', {
      secret: totpSecret.value,
      code: totpCode.value,
    })
    totpSecret.value = ''
    totpCode.value = ''
    totpEnabled.value = true
    toastOk(t('settings.twofaEnabled'))
  } catch (e: any) {
    totpErr.value = e?.message || t('settings.twofaFailed')
  } finally {
    totpBusy.value = false
  }
}

async function disableTOTP() {
  try {
    await api.post('/api/v1/admin/disable-totp')
    totpEnabled.value = false
    toastOk(t('settings.twofaDisabled'))
  } catch (e: any) {
    toastErr(e?.message || t('settings.saveFailed'))
  }
}

// ---------- 客户端版本控制 ----------
const settings = reactive<ServerSettings>({ minimum_client_version: '', blocked_versions: '' })
const blockedText = ref('')
const versionErr = ref('')
const versionBusy = ref(false)

async function loadSettings() {
  try {
    const s = await api.get<ServerSettings>('/api/v1/admin/settings')
    Object.assign(settings, s)
    blockedText.value = s.blocked_versions || ''
  } catch {
    /* 设置加载失败静默,表单留空 */
  }
}

async function saveSettings() {
  versionErr.value = ''
  // 校验 blocked_versions JSON
  if (blockedText.value.trim()) {
    try {
      JSON.parse(blockedText.value)
    } catch {
      versionErr.value = t('settings.configInvalid')
      return
    }
  }
  versionBusy.value = true
  try {
    await api.put('/api/v1/admin/settings', {
      key: 'minimum_client_version',
      value: settings.minimum_client_version || '',
    })
    await api.put('/api/v1/admin/settings', {
      key: 'blocked_versions',
      value: blockedText.value.trim(),
    })
    toastOk(t('settings.configSaved'))
  } catch (e: any) {
    versionErr.value = e?.message || t('settings.saveFailed')
  } finally {
    versionBusy.value = false
  }
}

// ---------- 签名密钥 ----------
const keys = ref<SigningKey[]>([])

async function loadKeys() {
  try {
    const res = await api.get<{ items: SigningKey[] }>('/api/v1/admin/signing-keys')
    keys.value = res.items || []
  } catch {
    keys.value = []
  }
}

onMounted(() => {
  loadSettings()
  loadKeys()
})
</script>
