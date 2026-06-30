import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Modelith',
  tagline: 'Author, validate, and render domain models by talking to an AI agent.',
  favicon: 'img/favicon.svg',

  future: {
    v4: true,
  },

  url: 'https://modelith.sh',
  baseUrl: '/',

  onBrokenLinks: 'throw',
  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },
  themes: [
    '@docusaurus/theme-mermaid',
    [
      require.resolve('@easyops-cn/docusaurus-search-local'),
      {
        hashed: true,
        docsRouteBasePath: '/',
        docsDir: '../docs',
        indexBlog: false,
        highlightSearchTermsOnTargetPage: true,
      },
    ],
  ],

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          path: '../docs',
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/stacklok/modelith/edit/main/',
          exclude: ['**/_*'],
          showLastUpdateTime: true,
          showLastUpdateAuthor: true,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    colorMode: {
      defaultMode: 'dark',
      disableSwitch: true,
      respectPrefersColorScheme: false,
    },
    navbar: {
      title: 'Modelith',
      style: 'dark',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Docs',
        },
        {
          href: 'https://github.com/stacklok/modelith',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      logo: {
        alt: 'Stacklok',
        src: 'img/stacklok-logo.svg',
        href: 'https://stacklok.com',
        width: 120,
      },
      links: [
        {
          title: 'Docs',
          items: [
            {label: 'Getting Started', to: '/getting-started'},
            {label: 'Schema Reference', to: '/schema-reference'},
            {label: 'CLI', to: '/cli'},
            {label: 'GitHub Action', to: '/github-action'},
          ],
        },
        {
          title: 'More',
          items: [
            {label: 'GitHub', href: 'https://github.com/stacklok/modelith'},
            {label: 'Releases', href: 'https://github.com/stacklok/modelith/releases'},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Stacklok, Inc. Apache 2.0 licensed.`,
    },
    prism: {
      // Dark-only site — only need the dark theme.
      theme: prismThemes.dracula,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'go', 'yaml', 'json'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
