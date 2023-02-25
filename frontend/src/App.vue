<template>
<!--  <HelloWorld msg="Welcome to Your Vue.js App"/>-->
  <div class="common-layout">
    <el-container>
      <el-main>
        <Table/>
      </el-main>
    </el-container>
  </div>

</template>

<script>
import Table from './components/Table.vue'

export default {
  name: 'App',
  components: {
    Table
  }
}
</script>

<style lang="scss">
@import "./src/element-variables";

#app {
  width: $app-window-width;
  height: 100vh;
}
</style>