export default function(eleventyConfig) {
  eleventyConfig.addPassthroughCopy({ public: '/' })
  eleventyConfig.addPassthroughCopy('assets/css')
  eleventyConfig.addPassthroughCopy('assets/js')

  return {
    dir: {
      input: 'pages',
      output: '_site'
    },
    templateFormats: ['html'],
    htmlTemplateEngine: 'liquid'
  }
}
