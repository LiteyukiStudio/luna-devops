import * as path from 'node:path';
import { defineConfig } from '@rspress/core';

export default defineConfig({
  root: path.join(__dirname, 'docs'),
  title: 'Luna DevOps',
  description: 'Deploy and operate applications in a few clear steps, built for small teams and businesses.',
  lang: 'zh',
  icon: '/luna-devops-logo.svg',
  logo: {
    light: '/luna-devops-logo.svg',
    dark: '/luna-devops-logo.svg',
  },
  locales: [
    {
      lang: 'zh',
      label: '简体中文',
      title: 'Luna DevOps',
      description: '轻松几步部署和管理应用，面向小型团队和企业的应用交付控制台。',
    },
    {
      lang: 'en',
      label: 'English',
      title: 'Luna DevOps',
      description: 'Deploy and operate applications in a few clear steps, built for small teams and businesses.',
    },
  ],
  themeConfig: {
    socialLinks: [
      {
        icon: 'github',
        mode: 'link',
        content: 'https://github.com/LiteyukiStudio/luna-devops',
      },
    ],
  },
});
