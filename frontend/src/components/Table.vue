<template>
  <el-row class="table-wrapper">
    <el-row class="top-panel">
      <el-form-item label="Папка">
        <el-input
            v-model="path"
            placeholder="Выберите папку"
            class="input-with-select"
            size="small"
        >
          <template #append>
            <el-button size="small" type="primary" @click.prevent="openFolderDialog()">
              <el-icon size="small">
                <Upload/>
              </el-icon>
            </el-button>
          </template>
        </el-input>
      </el-form-item>

    </el-row>
    <el-table :data="tasks">
      <el-table-column prop="link" label="Ссылка на *.m3u8" width="700">
        <template #default="scope">
          <el-row  v-if="scope.row.isStarted" class="small progress-wrapper">
            <el-popover
                placement="top-start"
                :width="600"
                trigger="hover"
                :content="scope.row.link"
            >
              <template #reference>
                <el-button plain size="small" @click="copyText(scope.row.link)">
                  <el-icon size="small">
                    <Link/>
                  </el-icon>
                </el-button>
              </template>
            </el-popover>
            <el-progress :percentage="formatNumber(scope.row.progress,2)"/>
          </el-row>
          <el-input v-else v-model="scope.row.link" size="small"/>
        </template>
      </el-table-column>
      <el-table-column prop="path" label="Название файла" width="150">
        <template #default="scope">
          <el-row  v-if="scope.row.isStarted" class="small">
            <div>{{ scope.row.fileName }}</div>
          </el-row>
          <el-input
              v-else
              v-model="scope.row.fileName"
              placeholder="Please input"
              size="small"
          >
          </el-input>
        </template>
      </el-table-column>
      <el-table-column label="" width="120">
        <template #default="scope">
          <el-row v-if="!scope.row.isCompleted" class="small">
            <el-button v-if="!scope.row.isPause && scope.row.isStarted" size="small" plain type="primary" @click.prevent="pause(scope.$index);">
              <el-icon size="small">
                <VideoPause/>
              </el-icon>
            </el-button>
            <el-button v-else size="small" plain type="success" @click.prevent="start(scope.$index);">
              <el-icon size="small">
                <VideoPlay/>
              </el-icon>
            </el-button>
            <el-popconfirm
                title="Удалить?"
                @confirm="deleteRow(scope.$index);"
                confirm-button-text="Да"
                cancel-button-text="Нет"
                width="200"
            >
              <template #reference>
                <el-button size="small" plain type="danger">
                  <el-icon size="small">
                    <Delete/>
                  </el-icon>
                </el-button>
              </template>
            </el-popconfirm>
          </el-row>
        </template>
      </el-table-column>
    </el-table>
    <el-row class="mb-4 bottom-panel">
      <el-button circle plain size="small" @click="onAddTask('')">
        <el-icon>
          <Plus/>
        </el-icon>
      </el-button>
    </el-row>
  </el-row>
</template>

<script lang="ts" setup>
import {ref, onMounted, vShow} from 'vue'
import dayjs from 'dayjs'
import { ElNotification } from 'element-plus'
import { copyTextToClipboard } from "../utils/copyToClipboard";
import { formatNumber } from "../utils/numbers";
import {
  GetDirPath, GetFileName, OpenDirectoryDialog, GetTasksProgress, PauseTask, StartTask, RemoveTask
} from "../../wailsjs/go/main/App";

const tasks = ref([
  // {
  //   link: 'https://s5.playep.pro/content/stream/films/sherlock.s04e03_208159/hls/720/index.m3u8',
  //   path: 'C:\\Users\\Zhekan\\GolandProjects\\uploader_with_ui\\main.ts',
  //   isUpload: false,
  //   isPause: false,
  //   progress: 0,
  // }
])

const path = ref('')
const defaultFileName = ref('')


const copyText = (text: string) => {
  copyTextToClipboard(text);

  ElNotification({
    title: 'Скопирована ссылка',
    message: text,
  })
}

const alert = (text: string) => {
  window.alert(text);
}

const openFolderDialog = async () => {
  path.value = await OpenDirectoryDialog(path.value)
}

const start = async (index: number) => {
  const item = tasks.value[index]
  item.isPause = false;
  item.isStarted = true;

  const filePath = item.folder + "\\" + item.fileName
  await StartTask(item.link,filePath)
}

const pause = async (index: number) => {
  const item = tasks.value[index]
  item.isPause = true;

  await PauseTask(item.link)
}

const deleteRow = async (index: number) => {
  const item = tasks.value[index]

  if (item.isStarted) {
    await RemoveTask(item.link)
  }

  tasks.value.splice(index, 1)
}

const sleep = ms => new Promise(r => setTimeout(r, ms));

const watchProgress = async () => {
  while (true) {
    await sleep(1000);

    const tasksUrlsList = []
    for (let task of tasks.value) {
      if (!task.isCompleted && task.isStarted) {
        tasksUrlsList.push(task.link)
      }
    }

    if (tasksUrlsList.length > 0) {
      const tasksProgress = await GetTasksProgress(tasksUrlsList)

      if(Array.isArray(tasksProgress)) {
        for (let taskProgress of tasksProgress) {
          for (let task of tasks.value) {
            if (taskProgress.url === task.link) {
              task.progress = taskProgress.progress
              task.status = taskProgress.status

              if (taskProgress.status === 6) {
                task.isCompleted = true

                await RemoveTask(taskProgress.url)

              }

              break
            }
          }
        }
      }
    }



  }
}

const onAddTask = (link:string = '') => {
  let i = 0

  while (true) {
    let fileName = defaultFileName.value

    if (i > 0) {
      fileName = fileName.replace('.ts',`_(${i}).ts`)
    }

    if (!tasks.value.some((element) => element.fileName === fileName)) {
      tasks.value.push({
        link,
        fileName,
        folder: path.value,
        isStarted: false,
        isPause: false,
        isCompleted: false,
        progress: 0,
        status: 0,
      })

      break
    }

    i++
  }
}

onMounted( async () =>  {
  path.value = await GetDirPath()
  defaultFileName.value = await GetFileName()

  //onAddTask('https://s5.playep.pro/content/stream/films/sherlock.s04e03_208159/hls/360/index.m3u8')

  onAddTask('https://river-6-602.rutube.ru/hls-vod/apo_r5IUOiaA8Miy-MZA6Q/1678612324/1788/0x5000c500c3d6b0cf/153f3585c65548d6a406354b63fec2d1.mp4.m3u8?i=256x144_551')
  onAddTask('https://river-6-602.rutube.ru/hls-vod/HAr5V-GCuEtUudsWFMb9tg/1678612324/1798/0x5000c500c4c4ec6c/9a2daeb31e074e0e866868b082b9e9ef.mp4.m3u8?i=856x480_1620')
  onAddTask('https://river-6-602.rutube.ru/hls-vod/0Ifr0z0YtPvNzsAhi662UA/1678612324/1816/0x5000c500db51716d/bbdbdee8cd5543919cdd1e78689c859c.mp4.m3u8?i=1280x720_3174')
  onAddTask('https://river-6-602.rutube.ru/hls-vod/POr3qesMEv8ticRiAVrB4Q/1678612324/1776/0x5000c500c923de22/dd50dc18eb0e43e4ba1d0150c22ec30f.mp4.m3u8?i=1920x1080_5126')

  onAddTask('https://river-3-301.rutube.ru/hls-vod/nmI63TkJDFaxLaUq3pxWVw/1678612424/1718/0x5000039b58c89c43/1b2c4c27521a444c98e78c7304078843.mp4.m3u8?i=512x288_762')
  onAddTask('https://river-3-301.rutube.ru/hls-vod/1bPkGDL-m9VoeJCXYiqQcw/1678612424/1784/0x5000c500c99eff80/6d625d43d1604f059f50c98a4448586a.mp4.m3u8?i=1280x720_1928')

  watchProgress()
})

</script>

<style lang="scss">
@import "./src/element-variables";

/* используем SCSS здесь */
.table-wrapper {
  .bottom-panel {
    margin: 10px;
    justify-items: center;
    align-items: center;
    width: 100%;
    flex-direction: column;
  }

  .top-panel {
    margin: 10px;
    width: 100%;
    display: block;
  }

  .small {
    --el-input-height: var(--el-component-size-small);
    font-size: 12px;
  }

  .el-progress--line {
    width: 325px;
  }
  .el-progress__text {
    font-size: 12px!important;
    min-width: 30px;
  }

  .progress-wrapper {
    justify-content: space-between;
  }
}

</style>