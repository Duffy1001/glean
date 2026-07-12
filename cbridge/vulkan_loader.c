#include <vulkan/vulkan.h>

#ifdef _WIN32
#include <windows.h>

static HMODULE vulkan_library;
static PFN_vkGetInstanceProcAddr vulkan_get_instance_proc_addr;
static PFN_vkGetPhysicalDeviceFeatures2 vulkan_get_physical_device_features2;
static PFN_vkCmdCopyBuffer vulkan_cmd_copy_buffer;
static int vulkan_load_attempted;

static void vulkan_loader_init(void) {
    if (vulkan_load_attempted) return;
    vulkan_load_attempted = 1;
    vulkan_library = LoadLibraryExW(L"vulkan-1.dll", NULL, LOAD_LIBRARY_SEARCH_SYSTEM32);
    if (vulkan_library) {
        vulkan_get_instance_proc_addr = (PFN_vkGetInstanceProcAddr)GetProcAddress(vulkan_library, "vkGetInstanceProcAddr");
        vulkan_get_physical_device_features2 = (PFN_vkGetPhysicalDeviceFeatures2)GetProcAddress(vulkan_library, "vkGetPhysicalDeviceFeatures2");
        vulkan_cmd_copy_buffer = (PFN_vkCmdCopyBuffer)GetProcAddress(vulkan_library, "vkCmdCopyBuffer");
    }
}
#else
#include <dlfcn.h>

static void * vulkan_library;
static PFN_vkGetInstanceProcAddr vulkan_get_instance_proc_addr;
static PFN_vkGetPhysicalDeviceFeatures2 vulkan_get_physical_device_features2;
static PFN_vkCmdCopyBuffer vulkan_cmd_copy_buffer;
static int vulkan_load_attempted;

static void vulkan_loader_init(void) {
    if (vulkan_load_attempted) return;
    vulkan_load_attempted = 1;
    vulkan_library = dlopen("libvulkan.so.1", RTLD_NOW | RTLD_LOCAL);
    if (vulkan_library) {
        vulkan_get_instance_proc_addr = (PFN_vkGetInstanceProcAddr)dlsym(vulkan_library, "vkGetInstanceProcAddr");
        vulkan_get_physical_device_features2 = (PFN_vkGetPhysicalDeviceFeatures2)dlsym(vulkan_library, "vkGetPhysicalDeviceFeatures2");
        vulkan_cmd_copy_buffer = (PFN_vkCmdCopyBuffer)dlsym(vulkan_library, "vkCmdCopyBuffer");
    }
}
#endif

int glean_vulkan_loader_available(void) {
    vulkan_loader_init();
    return vulkan_get_instance_proc_addr != NULL &&
           vulkan_get_physical_device_features2 != NULL &&
           vulkan_cmd_copy_buffer != NULL;
}

VKAPI_ATTR PFN_vkVoidFunction VKAPI_CALL vkGetInstanceProcAddr(VkInstance instance, const char * name) {
    vulkan_loader_init();
    if (!vulkan_get_instance_proc_addr) return NULL;
    return vulkan_get_instance_proc_addr(instance, name);
}

VKAPI_ATTR void VKAPI_CALL vkGetPhysicalDeviceFeatures2(VkPhysicalDevice physical_device, VkPhysicalDeviceFeatures2 * features) {
    vulkan_loader_init();
    if (vulkan_get_physical_device_features2) vulkan_get_physical_device_features2(physical_device, features);
}

VKAPI_ATTR void VKAPI_CALL vkCmdCopyBuffer(VkCommandBuffer command_buffer, VkBuffer source, VkBuffer destination, uint32_t region_count, const VkBufferCopy * regions) {
    vulkan_loader_init();
    if (vulkan_cmd_copy_buffer) vulkan_cmd_copy_buffer(command_buffer, source, destination, region_count, regions);
}
