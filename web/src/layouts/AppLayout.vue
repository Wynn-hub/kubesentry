<template>
  <el-container style="height: 100vh">
    <el-aside width="200px">
      <div style="padding: 16px; font-weight: 700">KubeSentry</div>
      <el-menu :default-active="$route.path" router>
        <el-menu-item index="/">{{ $t('nav.overview') }}</el-menu-item>
        <el-menu-item index="/policies">{{ $t('nav.policies') }}</el-menu-item>
        <el-menu-item index="/policygroups">{{ $t('nav.groups') }}</el-menu-item>
        <el-menu-item index="/exceptions">{{ $t('nav.exceptions') }}</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header style="display: flex; justify-content: flex-end; align-items: center">
        <el-select :model-value="locale" style="width: 110px" @change="onLocale">
          <el-option label="中文" value="zh-CN" />
          <el-option label="English" value="en-US" />
        </el-select>
      </el-header>
      <el-main><router-view /></el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { setLocale } from '../locales'

const { locale } = useI18n()
const onLocale = (l: 'zh-CN' | 'en-US') => setLocale(l)
</script>
