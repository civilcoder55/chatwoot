<script setup>
import { reactive, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { useVuelidate } from '@vuelidate/core';
import { required, url } from '@vuelidate/validators';
import { useAlert } from 'dashboard/composables';
import { isPhoneE164 } from 'shared/helpers/Validators';
import { useStore, useMapGetter } from 'dashboard/composables/store';

import PageHeader from '../../SettingsSubPageHeader.vue';
import Input from 'dashboard/components-next/input/Input.vue';
import NextButton from 'dashboard/components-next/button/Button.vue';

const { t } = useI18n();
const store = useStore();
const router = useRouter();

const state = reactive({
  phoneNumber: '',
  apiKey: '',
});

const uiFlags = useMapGetter('inboxes/getUIFlags');

const validationRules = {
  phoneNumber: { required, isPhoneE164 },
  apiKey: { required },
};

const v$ = useVuelidate(validationRules, state);
const isSubmitDisabled = computed(() => v$.value.$invalid);

const formErrors = computed(() => ({
  phoneNumber: v$.value.phoneNumber?.$error
    ? t('INBOX_MGMT.ADD.SIP.PHONE_NUMBER.ERROR')
    : '',
  apiKey: v$.value.apiKey?.$error
    ? t('INBOX_MGMT.ADD.SIP.API_KEY.ERROR')
    : '',
}));

async function createChannel() {
  const isFormValid = await v$.value.$validate();
  if (!isFormValid) return;

  try {
    const channel = await store.dispatch('inboxes/createSipChannel', {
      name: `SIP (${state.phoneNumber})`,
      sip: {
        phone_number: state.phoneNumber,
        provider_config: {
          api_key: state.apiKey,
        },
      },
    });

    router.replace({
      name: 'settings_inboxes_add_agents',
      params: { page: 'new', inbox_id: channel.id },
    });
  } catch (error) {
    useAlert(
      error.response?.data?.message || t('INBOX_MGMT.ADD.SIP.API.ERROR_MESSAGE')
    );
  }
}
</script>

<template>
  <div class="overflow-auto col-span-6 p-6 w-full h-full">
    <PageHeader
      :header-title="t('INBOX_MGMT.ADD.SIP.TITLE')"
      :header-content="t('INBOX_MGMT.ADD.SIP.DESC')"
    />

    <form
      class="flex flex-col gap-4 flex-wrap mx-0"
      @submit.prevent="createChannel"
    >
      <Input
        v-model="state.phoneNumber"
        :label="t('INBOX_MGMT.ADD.SIP.PHONE_NUMBER.LABEL')"
        :placeholder="t('INBOX_MGMT.ADD.SIP.PHONE_NUMBER.PLACEHOLDER')"
        :message="formErrors.phoneNumber"
        :message-type="formErrors.phoneNumber ? 'error' : 'info'"
        @blur="v$.phoneNumber?.$touch"
      />

      <Input
        v-model="state.apiKey"
        type="password"
        :label="t('INBOX_MGMT.ADD.SIP.API_KEY.LABEL')"
        :placeholder="t('INBOX_MGMT.ADD.SIP.API_KEY.PLACEHOLDER')"
        :message="formErrors.apiKey"
        :message-type="formErrors.apiKey ? 'error' : 'info'"
        @blur="v$.apiKey?.$touch"
      />

      <div>
        <NextButton
          :is-loading="uiFlags.isCreating"
          :disabled="isSubmitDisabled"
          :label="t('INBOX_MGMT.ADD.SIP.SUBMIT_BUTTON')"
          type="submit"
        />
      </div>
    </form>
  </div>
</template>
