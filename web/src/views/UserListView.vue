<template>
  <PageHeader :title="$t('user.title')" :subtitle="$t('user.subtitle')"><el-button type="primary" @click="dialog = true">{{ $t('common.create') }}</el-button></PageHeader>
  <el-table v-loading="loading" :data="items">
    <el-table-column prop="username" :label="$t('auth.username')" />
    <el-table-column prop="role" :label="$t('user.role')" width="150" />
    <el-table-column prop="tenantId" :label="$t('user.tenant')" width="120"><template #default="{ row }">{{ tenantName(row.tenantId) }}</template></el-table-column>
    <el-table-column prop="status" :label="$t('common.status')" width="110" />
    <el-table-column :label="$t('common.actions')" width="250"><template #default="{ row }">
      <template v-if="canManage(row)">
        <el-switch :model-value="row.status === 'active'" @change="toggle(row)" />
        <el-button link @click="openReset(row)">{{ $t('user.resetPassword') }}</el-button>
        <el-button v-if="row.username !== 'admin'" link type="danger" @click="remove(row)">{{ $t('common.delete') }}</el-button>
      </template>
    </template></el-table-column>
  </el-table>
  <el-dialog v-model="dialog" :title="$t('user.createTitle')" width="460px">
    <el-form label-position="top" @submit.prevent="save">
      <el-form-item :label="$t('auth.username')"><el-input v-model="form.username" autofocus /></el-form-item>
      <el-form-item v-if="systemAdmin" :label="$t('user.role')"><el-select v-model="form.role"><el-option value="tenant_admin" :label="$t('user.tenantAdmin')" /><el-option value="viewer" :label="$t('user.viewer')" /></el-select></el-form-item>
      <el-form-item v-if="systemAdmin" :label="$t('user.tenant')"><el-select v-model="form.tenantId"><el-option v-for="tenant in activeTenants" :key="tenant.id" :value="tenant.id!" :label="tenant.name" /></el-select></el-form-item>
    </el-form>
    <template #footer><el-button @click="dialog = false">{{ $t('common.cancel') }}</el-button><el-button type="primary" :loading="saving" @click="save">{{ $t('common.create') }}</el-button></template>
  </el-dialog>
  <el-dialog v-model="resetDialog" :title="$t('user.resetPassword')" width="420px"><el-input v-model="newPassword" type="password" show-password /><template #footer><el-button @click="resetDialog = false">{{ $t('common.cancel') }}</el-button><el-button type="primary" @click="resetPassword">{{ $t('common.confirm') }}</el-button></template></el-dialog>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { createUser, deleteUser, listTenants, listUsers, resetUserPassword, setUserStatus, type Tenant, type User } from '../api/identity'
import { authState } from '../auth/session'
import PageHeader from '../components/PageHeader.vue'
const { t } = useI18n(), items = ref<User[]>([]), tenants = ref<Tenant[]>([]), loading = ref(false), saving = ref(false), dialog = ref(false), resetDialog = ref(false), newPassword = ref(''), selected = ref<User>()
const systemAdmin = computed(() => authState.me?.role === 'system_admin'), activeTenants = computed(() => tenants.value.filter(t => t.status === 'active'))
const form = reactive({ username: '', role: 'viewer' as 'tenant_admin' | 'viewer', tenantId: undefined as string | undefined })
const tenantName = (id?: string) => tenants.value.find(t => t.id === id)?.name ?? '-'
const canManage = (user: User) => systemAdmin.value || user.role === 'viewer'
async function load() { loading.value = true; try { items.value = await listUsers() ?? []; if (systemAdmin.value) tenants.value = await listTenants() ?? [] } catch (e) { ElMessage.error((e as Error).message) } finally { loading.value = false } }
async function save() { saving.value = true; try { await createUser({ username: form.username, role: systemAdmin.value ? form.role : 'viewer', tenantId: systemAdmin.value ? form.tenantId : authState.me?.tenantId }); dialog.value = false; Object.assign(form, { username: '', role: 'viewer', tenantId: undefined }); await load() } catch (e) { ElMessage.error((e as Error).message) } finally { saving.value = false } }
async function toggle(row: User) { try { await setUserStatus(row.id!, row.status === 'active' ? 'disabled' : 'active'); await load() } catch (e) { ElMessage.error((e as Error).message) } }
function openReset(row: User) { selected.value = row; newPassword.value = ''; resetDialog.value = true }
async function resetPassword() { try { await resetUserPassword(selected.value!.id!, newPassword.value); resetDialog.value = false } catch (e) { ElMessage.error((e as Error).message) } }
async function remove(row: User) { try { await ElMessageBox.confirm(t('common.deleteConfirm', { name: row.username })); await deleteUser(row.id!); await load() } catch (e) { if (e !== 'cancel') ElMessage.error((e as Error).message) } }
onMounted(load)
</script>
